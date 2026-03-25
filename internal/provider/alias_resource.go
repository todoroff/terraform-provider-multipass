package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/todoroff/terraform-provider-multipass/internal/models"
	"github.com/todoroff/terraform-provider-multipass/internal/multipasscli"
)

var (
	_ resource.Resource                = (*aliasResource)(nil)
	_ resource.ResourceWithConfigure   = (*aliasResource)(nil)
	_ resource.ResourceWithImportState = (*aliasResource)(nil)
)

// NewAliasResource instantiates the resource.
func NewAliasResource() resource.Resource {
	return &aliasResource{}
}

type aliasResource struct {
	client multipasscli.Client
}

type aliasResourceModel struct {
	ID               types.String `tfsdk:"id"`
	Name             types.String `tfsdk:"name"`
	Instance         types.String `tfsdk:"instance"`
	Command          types.String `tfsdk:"command"`
	WorkingDirectory types.String `tfsdk:"working_directory"`
}

func (r *aliasResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alias"
}

func (r *aliasResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages Multipass CLI aliases.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Alias name accessible on the host.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"instance": schema.StringAttribute{
				Required:    true,
				Description: "Target Multipass instance.",
			},
			"command": schema.StringAttribute{
				Required:    true,
				Description: "Command to execute inside the instance.",
			},
			"working_directory": schema.StringAttribute{
				Optional:            true,
				Description:         "Working directory inside the instance. The command is wrapped to cd into this directory before execution.",
				MarkdownDescription: "Working directory inside the instance. The command is wrapped with `cd <dir> && exec <command>` automatically.",
			},
		},
	}
}

func (r *aliasResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data := req.ProviderData.(providerData)
	r.client = data.client
}

func (r *aliasResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError("Client not configured", "Multipass client is nil.")
		return
	}

	var plan aliasResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.CreateAlias(ctx, aliasFromModel(&plan)); err != nil {
		resp.Diagnostics.AddError("Failed to create alias", err.Error())
		return
	}

	plan.ID = plan.Name
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *aliasResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError("Client not configured", "Multipass client is nil.")
		return
	}

	var state aliasResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	aliases, err := r.client.ListAliases(ctx, false)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list aliases", err.Error())
		return
	}

	name := state.Name.ValueString()
	for _, alias := range aliases {
		if alias.Name == name {
			state.ID = types.StringValue(name)
			state.Instance = types.StringValue(alias.Instance)
			// command and working_directory are kept from state because
			// the API returns the wrapped command which doesn't match
			// the user's original values.
			resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
			return
		}
	}

	resp.State.RemoveResource(ctx)
}

func (r *aliasResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError("Client not configured", "Multipass client is nil.")
		return
	}

	var plan aliasResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Multipass aliases cannot be updated in-place; delete then recreate.
	if err := r.client.DeleteAlias(ctx, plan.Name.ValueString()); err != nil && err != multipasscli.ErrNotFound {
		resp.Diagnostics.AddError("Failed to delete alias for update", err.Error())
		return
	}

	if err := r.client.CreateAlias(ctx, aliasFromModel(&plan)); err != nil {
		resp.Diagnostics.AddError("Failed to recreate alias", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *aliasResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if r.client == nil {
		return
	}

	var state aliasResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if err := r.client.DeleteAlias(ctx, state.Name.ValueString()); err != nil && err != multipasscli.ErrNotFound {
		resp.Diagnostics.AddError("Failed to delete alias", err.Error())
	}
}

func (r *aliasResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

func aliasFromModel(m *aliasResourceModel) models.Alias {
	return models.Alias{
		Name:             m.Name.ValueString(),
		Instance:         m.Instance.ValueString(),
		Command:          m.Command.ValueString(),
		WorkingDirectory: valueOrEmpty(m.WorkingDirectory),
	}
}
