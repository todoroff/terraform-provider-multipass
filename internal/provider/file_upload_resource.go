package provider

import (
	"context"
	"errors"
	"fmt"
	"os"
	guestpath "path"
	"path/filepath"
	"strings"

	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	stringvalidator "github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
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
	client         multipasscli.Client
	commandTimeout time.Duration
}

type fileUploadResourceModel struct {
	ID                  types.String   `tfsdk:"id"`
	Instance            types.String   `tfsdk:"instance"`
	Destination         types.String   `tfsdk:"destination"`
	ResolvedDestination types.String   `tfsdk:"resolved_destination"`
	Directory           types.Bool     `tfsdk:"directory"`
	Source              types.String   `tfsdk:"source"`
	Content             types.String   `tfsdk:"content"`
	Recursive           types.Bool     `tfsdk:"recursive"`
	CreateParents       types.Bool     `tfsdk:"create_parents"`
	ContentHash         types.String   `tfsdk:"content_hash"`
	Timeouts            timeouts.Value `tfsdk:"timeouts"`
}

func (r *fileUploadResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_file_upload"
}

func (r *fileUploadResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	oneOf := []path.Expression{
		path.MatchRelative().AtParent().AtName("source"),
		path.MatchRelative().AtParent().AtName("content"),
	}

	resp.Schema = schema.Schema{
		Description: "Uploads local files, inline content, or directories to Multipass instances via `multipass transfer`.",
		Attributes: map[string]schema.Attribute{
			"resolved_destination": schema.StringAttribute{
				Computed:      true,
				Description:   "Exact remote path owned by this upload, including the child name when destination is a directory.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"directory": schema.BoolAttribute{
				Computed:      true,
				Description:   "Whether the owned payload is a directory. File deletion is never recursive.",
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
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
		Blocks: map[string]schema.Block{
			"timeouts": timeouts.Block(ctx, timeouts.Opts{
				Create: true,
				Update: true,
			}),
		},
	}
}

func (r *fileUploadResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	data := req.ProviderData.(providerData)
	r.client = data.client
	r.commandTimeout = data.commandTimeout
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

	if plan.Source.IsUnknown() || plan.Content.IsUnknown() || plan.Recursive.IsUnknown() {
		// Upstream resources can supply file inputs during apply. Explicitly
		// clear any hash copied from prior state by UseStateForUnknown.
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("content_hash"), types.StringUnknown())...)
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

	createTimeout, diags := plan.Timeouts.Create(ctx, r.commandTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, createTimeout)
	defer cancel()

	hashValue, diags := r.computeHash(&plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.transferUpload(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
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
		if errors.Is(err, multipasscli.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to verify instance", err.Error())
		return
	}

	if err := r.resolveLegacyUpload(ctx, &state); err != nil {
		resp.Diagnostics.AddError("Unknown upload ownership", err.Error())
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

	updateTimeout, diags := plan.Timeouts.Update(ctx, r.commandTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, updateTimeout)
	defer cancel()

	hashValue, diags := r.computeHash(&plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	resp.Diagnostics.Append(r.transferUpload(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	plan.ContentHash = types.StringValue(hashValue)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *fileUploadResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError("Client not configured", "The provider Multipass client was not configured.")
		return
	}

	var state fileUploadResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	instance := state.Instance.ValueString()
	if err := r.resolveLegacyUpload(ctx, &state); err != nil {
		resp.Diagnostics.AddError("Unknown upload ownership", err.Error())
		return
	}
	dest := state.ResolvedDestination.ValueString()
	if instance == "" || dest == "" {
		return
	}

	flags := "-f"
	if state.Directory.ValueBool() {
		flags = "-rf"
	}
	if err := r.client.Exec(ctx, instance, []string{"rm", flags, "--", dest}); err != nil {
		// CLI error text can describe a missing command, not a missing VM.
		if instances, listErr := r.client.ListInstances(ctx, true); listErr == nil {
			found := false
			for _, vm := range instances {
				found = found || vm.Name == instance
			}
			if !found {
				return
			}
		}
		resp.Diagnostics.AddError("Failed to remove remote path", err.Error())
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
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("resolved_destination"), parts[1])...)
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

// prepareLocalSource resolves the model into a form `multipass transfer` can
// consume. For file-based uploads it returns an absolute host path. For
// inline content it returns the bytes to pipe via stdin — avoiding a
// temporary file that snap-confined multipass installs cannot read.
func (r *fileUploadResource) prepareLocalSource(model *fileUploadResourceModel) (string, []byte, diag.Diagnostics) {
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
		return "", []byte(model.Content.ValueString()), diags
	}

	diags.AddError("Missing file data", "Either `source` or `content` must be provided.")
	return "", nil, diags
}

func (r *fileUploadResource) remotePathKind(ctx context.Context, instance, dest string) (string, error) {
	out, err := r.client.ExecCapture(ctx, instance, []string{"sh", "-c", `if [ -d "$1" ]; then printf directory; elif [ -e "$1" ] || [ -L "$1" ]; then printf file; else printf missing; fi`, "sh", dest})
	if err != nil {
		return "", err
	}
	kind := string(out)
	switch kind {
	case "directory", "file", "missing":
		return kind, nil
	}
	return "", fmt.Errorf("unexpected remote path type %q", kind)
}

func (r *fileUploadResource) resolveLegacyUpload(ctx context.Context, model *fileUploadResourceModel) error {
	if !hasStringValue(model.ResolvedDestination) {
		if model.Recursive.ValueBool() {
			return fmt.Errorf("legacy recursive upload does not record its owned path; re-import using <instance>:<exact-owned-path> before updating or deleting it")
		}
		dest := model.Destination.ValueString()
		if dest == "" {
			return fmt.Errorf("destination must be non-empty")
		}
		kind, err := r.remotePathKind(ctx, model.Instance.ValueString(), dest)
		if err != nil {
			return err
		}
		if kind == "directory" {
			base := "stream_output.dat"
			if hasStringValue(model.Source) {
				base = filepath.Base(model.Source.ValueString())
			}
			dest = guestpath.Join(dest, base)
		}
		model.ResolvedDestination = types.StringValue(dest)
		model.Directory = types.BoolValue(false)
	}
	if model.Directory.IsNull() || model.Directory.IsUnknown() {
		kind, err := r.remotePathKind(ctx, model.Instance.ValueString(), model.ResolvedDestination.ValueString())
		if err != nil {
			return err
		}
		model.Directory = types.BoolValue(kind == "directory")
	}
	dest := guestpath.Clean(model.ResolvedDestination.ValueString())
	if dest == "." || dest == "/" || dest == ".." {
		return fmt.Errorf("refusing ownership of broad destination %q", dest)
	}
	return nil
}

func (r *fileUploadResource) transferUpload(ctx context.Context, model *fileUploadResourceModel) (diags diag.Diagnostics) {
	src, content, d := r.prepareLocalSource(model)
	diags.Append(d...)
	if diags.HasError() {
		return
	}
	directory := false
	base := "stream_output.dat"
	if src != "" {
		info, err := os.Stat(src)
		if err != nil {
			diags.AddError("Invalid source", err.Error())
			return
		}
		directory = info.IsDir()
		base = filepath.Base(src)
	}
	dest := model.ResolvedDestination.ValueString()
	if !hasStringValue(model.ResolvedDestination) {
		if hasStringValue(model.ID) {
			if err := r.resolveLegacyUpload(ctx, model); err != nil {
				diags.AddError("Unknown upload ownership", err.Error())
				return
			}
			dest = model.ResolvedDestination.ValueString()
		} else {
			dest = model.Destination.ValueString()
			if dest == "" {
				diags.AddError("Invalid destination", "Destination must be non-empty")
				return
			}
			kind, err := r.remotePathKind(ctx, model.Instance.ValueString(), dest)
			if err != nil {
				diags.AddError("Failed to resolve upload destination", err.Error())
				return
			}
			if kind == "directory" {
				dest = guestpath.Join(dest, base)
			}
		}
	}
	if !model.Directory.IsNull() && !model.Directory.IsUnknown() && model.Directory.ValueBool() != directory {
		diags.AddError("Upload payload type changed", "Replace the upload resource when changing between a file and a directory.")
		return
	}
	model.ResolvedDestination = types.StringValue(dest)
	model.Directory = types.BoolValue(directory)
	if err := r.resolveLegacyUpload(ctx, model); err != nil {
		diags.AddError("Invalid upload ownership", err.Error())
		return
	}
	instance := model.Instance.ValueString()
	kind, err := r.remotePathKind(ctx, instance, dest)
	if err != nil {
		diags.AddError("Failed to inspect owned path", err.Error())
		return
	}
	if (directory && kind == "file") || (!directory && kind == "directory") {
		diags.AddError("Destination type changed", "The owned path has the wrong file type; refusing to overwrite it.")
		return
	}
	opts := multipasscli.TransferOptions{Destination: instance + ":" + dest, Recursive: directory, Parents: model.CreateParents.ValueBool()}
	if content != nil {
		opts.Stdin = content
	} else {
		opts.Sources = []string{src}
	}
	if directory && kind == "directory" {
		// Transfer into a private staging directory, then merge contents at the exact owned root.
		out, err := r.client.ExecCapture(ctx, instance, []string{"mktemp", "-d", "/tmp/multipass-upload.XXXXXXXXXX"})
		if err != nil {
			diags.AddError("Failed to stage upload", err.Error())
			return
		}
		stage := strings.TrimSpace(string(out))
		if !strings.HasPrefix(stage, "/tmp/multipass-upload.") || guestpath.Dir(stage) != "/tmp" {
			diags.AddError("Invalid upload staging path", stage)
			return
		}
		defer func() {
			cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer cancel()
			if err := r.client.Exec(cleanupCtx, instance, []string{"rm", "-rf", "--", stage}); err != nil {
				diags.AddWarning("Failed to clean upload staging", fmt.Sprintf("Remove %q after checking the transfer: %v", stage, err))
			}
		}()
		opts.Destination = instance + ":" + stage
		if err := r.client.Transfer(ctx, opts); err != nil {
			diags.AddError("Failed to transfer file", err.Error())
			return
		}
		if err := r.client.Exec(ctx, instance, []string{"cp", "-a", "--", guestpath.Join(stage, base) + "/.", dest}); err != nil {
			diags.AddError("Failed to install upload", err.Error())
			return
		}
	} else if err := r.client.Transfer(ctx, opts); err != nil {
		diags.AddError("Failed to transfer file", err.Error())
		return
	}
	return
}
