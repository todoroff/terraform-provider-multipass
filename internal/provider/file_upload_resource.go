package provider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	stringvalidator "github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/todoroff/terraform-provider-multipass/internal/multipasscli"
)

var (
	_ resource.Resource                = (*fileUploadResource)(nil)
	_ resource.ResourceWithConfigure   = (*fileUploadResource)(nil)
	_ resource.ResourceWithModifyPlan  = (*fileUploadResource)(nil)
	_ resource.ResourceWithImportState = (*fileUploadResource)(nil)
)

// NewFileUploadResource registers the upload resource with the provider.
func NewFileUploadResource() resource.Resource {
	return &fileUploadResource{}
}

type fileUploadResource struct {
	client multipasscli.Client
}

type fileUploadResourceModel struct {
	ID            types.String `tfsdk:"id"`
	Instance      types.String `tfsdk:"instance"`
	Destination   types.String `tfsdk:"destination"`
	Source        types.String `tfsdk:"source"`
	Content       types.String `tfsdk:"content"`
	Recursive     types.Bool   `tfsdk:"recursive"`
	CreateParents types.Bool   `tfsdk:"create_parents"`
	ContentHash   types.String `tfsdk:"content_hash"`
}

func (r *fileUploadResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_file_upload"
}

func (r *fileUploadResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	oneOf := []path.Expression{
		path.MatchRelative().AtParent().AtName("source"),
		path.MatchRelative().AtParent().AtName("content"),
	}

	resp.Schema = schema.Schema{
		Description: "Uploads local files, inline content, or directories to Multipass instances via `multipass transfer`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				Description:         "Canonical identifier in the form `<instance>:<destination>`.",
				MarkdownDescription: "Canonical identifier in the form `<instance>:<destination>`.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"instance": schema.StringAttribute{
				Required:            true,
				Description:         "Target instance name.",
				MarkdownDescription: "Target Multipass instance name that must already exist.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"destination": schema.StringAttribute{
				Required:            true,
				Description:         "Absolute or relative path inside the instance (e.g. `/home/ubuntu/setup.sh`).",
				MarkdownDescription: "Absolute or relative path inside the instance where the file or directory will be written.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"source": schema.StringAttribute{
				Optional:            true,
				Description:         "Local path to the file or directory that should be uploaded.",
				MarkdownDescription: "Local path to the file or directory that should be uploaded. Conflicts with `content`.",
				Validators: []validator.String{
					stringvalidator.ExactlyOneOf(oneOf...),
				},
			},
			"content": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				Description:         "Inline file content to upload.",
				MarkdownDescription: "Inline file content to upload. Conflicts with `source`.",
				Validators: []validator.String{
					stringvalidator.ExactlyOneOf(oneOf...),
				},
			},
			"recursive": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
				Description:         "Whether to copy directories recursively (maps to `multipass transfer --recursive`).",
				MarkdownDescription: "Whether to copy directories recursively (maps to `multipass transfer --recursive`).",
			},
			"create_parents": schema.BoolAttribute{
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
				Description:         "Create destination parent directories as needed (maps to `multipass transfer --parents`).",
				MarkdownDescription: "Create destination parent directories as needed (maps to `multipass transfer --parents`).",
			},
			"content_hash": schema.StringAttribute{
				Computed:            true,
				Description:         "SHA256 hash of the payload sent to the instance. Changes trigger updates.",
				MarkdownDescription: "SHA256 hash of the payload sent to the instance. Changes trigger updates.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
		},
	}
}

func (r *fileUploadResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	data := req.ProviderData.(providerData)
	r.client = data.client
}

func (r *fileUploadResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}

	var plan fileUploadResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if plan.Source.IsUnknown() || plan.Content.IsUnknown() {
		resp.Diagnostics.AddAttributeError(
			path.Root("source"),
			"Unknown file inputs",
			"`source` or `content` must be known during planning.",
		)
		return
	}

	if plan.Instance.IsUnknown() || plan.Destination.IsUnknown() {
		resp.Diagnostics.AddError("Unknown target", "`instance` and `destination` must be known during planning.")
		return
	}

	if plan.Source.IsNull() && plan.Content.IsNull() {
		resp.Diagnostics.AddError("Missing file inputs", "Provide either `source` or `content`.")
		return
	}

	hashValue, diags := r.computeHash(&plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	plan.ContentHash = types.StringValue(hashValue)
	resp.Diagnostics.Append(resp.Plan.Set(ctx, &plan)...)
}

func (r *fileUploadResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError("Client not configured", "The provider Multipass client was not configured.")
		return
	}

	var plan fileUploadResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hashValue, diags := r.computeHash(&plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	path, cleanup, diags := r.prepareLocalSource(&plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cleanup != nil {
		defer cleanup()
	}

	target := fmt.Sprintf("%s:%s", plan.Instance.ValueString(), plan.Destination.ValueString())
	err := r.client.Transfer(ctx, multipasscli.TransferOptions{
		Sources:     []string{path},
		Destination: target,
		Recursive:   plan.Recursive.ValueBool(),
		Parents:     plan.CreateParents.ValueBool(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to transfer file", err.Error())
		return
	}

	plan.ID = types.StringValue(fmt.Sprintf("%s:%s", plan.Instance.ValueString(), plan.Destination.ValueString()))
	plan.ContentHash = types.StringValue(hashValue)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *fileUploadResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError("Client not configured", "The provider Multipass client was not configured.")
		return
	}

	var state fileUploadResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if state.Instance.IsNull() || state.Instance.ValueString() == "" {
		resp.State.RemoveResource(ctx)
		return
	}

	if _, err := r.client.GetInstance(ctx, state.Instance.ValueString()); err != nil {
		if err == multipasscli.ErrNotFound {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to verify instance", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *fileUploadResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError("Client not configured", "The provider Multipass client was not configured.")
		return
	}

	var plan fileUploadResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hashValue, diags := r.computeHash(&plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	path, cleanup, diags := r.prepareLocalSource(&plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if cleanup != nil {
		defer cleanup()
	}

	target := fmt.Sprintf("%s:%s", plan.Instance.ValueString(), plan.Destination.ValueString())
	err := r.client.Transfer(ctx, multipasscli.TransferOptions{
		Sources:     []string{path},
		Destination: target,
		Recursive:   plan.Recursive.ValueBool(),
		Parents:     plan.CreateParents.ValueBool(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Failed to transfer file", err.Error())
		return
	}

	plan.ContentHash = types.StringValue(hashValue)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *fileUploadResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if r.client == nil {
		return
	}

	var state fileUploadResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	instance := state.Instance.ValueString()
	dest := state.Destination.ValueString()
	if instance == "" || dest == "" {
		return
	}

	if err := r.client.Exec(ctx, instance, []string{"rm", "-rf", "--", dest}); err != nil {
		if cliErr, ok := err.(*multipasscli.CLIError); ok {
			resp.Diagnostics.AddWarning("Failed to remove remote path", cliErr.Error())
			return
		}
		resp.Diagnostics.AddWarning("Failed to remove remote path", err.Error())
	}
}

func (r *fileUploadResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	if resp.Diagnostics.HasError() {
		return
	}

	parts := strings.SplitN(req.ID, ":", 2)
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid import ID", "Expected `<instance>:<destination>`.")
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("instance"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("destination"), parts[1])...)
}

func (r *fileUploadResource) computeHash(model *fileUploadResourceModel) (string, diag.Diagnostics) {
	var diags diag.Diagnostics

	switch {
	case !model.Source.IsNull() && model.Source.ValueString() != "":
		hashValue, err := hashPath(model.Source.ValueString(), model.Recursive.ValueBool())
		if err != nil {
			diags.AddError("Failed to hash source", err.Error())
			return "", diags
		}
		return hashValue, diags
	case !model.Content.IsNull():
		value := model.Content.ValueString()
		return hashBytes([]byte(value)), diags
	default:
		diags.AddError("Missing file data", "Either `source` or `content` must be provided.")
		return "", diags
	}
}

func (r *fileUploadResource) prepareLocalSource(model *fileUploadResourceModel) (string, func(), diag.Diagnostics) {
	var diags diag.Diagnostics

	if !model.Source.IsNull() && model.Source.ValueString() != "" {
		abs, err := filepath.Abs(model.Source.ValueString())
		if err != nil {
			diags.AddError("Invalid source path", err.Error())
			return "", nil, diags
		}
		info, err := os.Stat(abs)
		if err != nil {
			diags.AddError("Invalid source path", err.Error())
			return "", nil, diags
		}
		if info.IsDir() && !model.Recursive.ValueBool() {
			diags.AddError("Directory transfer requires recursion", "Set `recursive = true` when `source` is a directory.")
			return "", nil, diags
		}
		return abs, nil, diags
	}

	if !model.Content.IsNull() {
		tmp, err := os.CreateTemp("", "multipass-file-*")
		if err != nil {
			diags.AddError("Failed to create temp file", err.Error())
			return "", nil, diags
		}
		if _, err := tmp.WriteString(model.Content.ValueString()); err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
			diags.AddError("Failed to write temp file", err.Error())
			return "", nil, diags
		}
		if err := tmp.Close(); err != nil {
			os.Remove(tmp.Name())
			diags.AddError("Failed to close temp file", err.Error())
			return "", nil, diags
		}
		return tmp.Name(), func() { os.Remove(tmp.Name()) }, diags
	}

	diags.AddError("Missing file data", "Either `source` or `content` must be provided.")
	return "", nil, diags
}
