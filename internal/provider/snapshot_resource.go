package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/todoroff/terraform-provider-multipass/internal/multipasscli"
)

var (
	_ resource.Resource                = (*snapshotResource)(nil)
	_ resource.ResourceWithConfigure   = (*snapshotResource)(nil)
	_ resource.ResourceWithImportState = (*snapshotResource)(nil)
)

// NewSnapshotResource instantiates the Multipass snapshot resource.
func NewSnapshotResource() resource.Resource {
	return &snapshotResource{}
}

type snapshotResource struct {
	client multipasscli.Client
}

type snapshotResourceModel struct {
	ID       types.String `tfsdk:"id"`
	Instance types.String `tfsdk:"instance"`
	Name     types.String `tfsdk:"name"`
	Comment  types.String `tfsdk:"comment"`
}

func (r *snapshotResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_snapshot"
}

func (r *snapshotResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a named snapshot for a Multipass instance.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
				Description: "Canonical identifier for the snapshot in the form `<instance>.<snapshot>`.",
			},
			"instance": schema.StringAttribute{
				Required:    true,
				Description: "Name of the Multipass instance to snapshot. The instance must be stopped.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"name": schema.StringAttribute{
				Optional:    true,
				Computed:    true,
				Description: "Snapshot name. If omitted, Multipass will auto-generate one (e.g., `snapshot1`). Changing forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"comment": schema.StringAttribute{
				Optional:    true,
				Description: "Optional comment associated with the snapshot. Changing forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
		},
	}
}

func (r *snapshotResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data := req.ProviderData.(providerData)
	r.client = data.client
}

func (r *snapshotResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError("Client not configured", "Multipass client is nil.")
		return
	}

	var plan snapshotResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	instance := plan.Instance.ValueString()
	name := ""
	if !plan.Name.IsNull() && !plan.Name.IsUnknown() {
		name = plan.Name.ValueString()
	}
	comment := ""
	if !plan.Comment.IsNull() && !plan.Comment.IsUnknown() {
		comment = plan.Comment.ValueString()
	}

	actualName, err := r.client.CreateSnapshot(ctx, instance, name, comment)
	if err != nil {
		resp.Diagnostics.AddError("Failed to create snapshot", err.Error())
		return
	}

	if actualName == "" {
		// Fallback to the requested name if parsing failed.
		actualName = name
	}

	id := fmt.Sprintf("%s.%s", instance, actualName)
	plan.ID = types.StringValue(id)
	plan.Instance = types.StringValue(instance)
	plan.Name = types.StringValue(actualName)

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *snapshotResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError("Client not configured", "Multipass client is nil.")
		return
	}

	var state snapshotResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	instance := state.Instance.ValueString()
	name := state.Name.ValueString()

	snapshots, err := r.client.ListSnapshots(ctx, instance)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list snapshots", err.Error())
		return
	}

	found := false
	for _, s := range snapshots {
		if s.Name == name {
			found = true
			// Keep comment in sync if present.
			state.Comment = types.StringValue(s.Comment)
			break
		}
	}

	if !found {
		tflog.Info(ctx, "Multipass snapshot no longer exists", map[string]any{
			"instance": instance,
			"name":     name,
		})
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *snapshotResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	// All updatable fields force replacement; no in-place updates.
	var plan snapshotResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *snapshotResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError("Client not configured", "Multipass client is nil.")
		return
	}

	var state snapshotResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	instance := state.Instance.ValueString()
	name := state.Name.ValueString()

	if err := r.client.DeleteSnapshot(ctx, instance, name, true); err != nil {
		if err == multipasscli.ErrNotFound {
			return
		}
		resp.Diagnostics.AddError("Failed to delete snapshot", err.Error())
	}
}

func (r *snapshotResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	// Expect ID in the form "instance.snapshot"
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	if resp.Diagnostics.HasError() {
		return
	}

	parts := strings.SplitN(req.ID, ".", 2)
	if len(parts) != 2 {
		resp.Diagnostics.AddError("Invalid import ID", "Expected <instance>.<snapshot>.")
		return
	}

	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("instance"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), parts[1])...)
}
