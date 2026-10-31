package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
	Context          types.String `tfsdk:"context"`
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
			"context": schema.StringAttribute{
				Computed:      true,
				Description:   "Multipass alias context captured at creation. Updates require this context to be selected with multipass prefer.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
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

	aliasContext, err := r.client.CurrentAliasContext(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read alias context", err.Error())
		return
	}
	plan.Context = types.StringValue(aliasContext)
	if err := r.client.CreateAlias(ctx, aliasFromModel(&plan)); err != nil {
		resp.Diagnostics.AddError("Failed to create alias", err.Error())
		return
	}

	plan.ID = types.StringValue(aliasContext + "." + plan.Name.ValueString())
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

	alias, err := r.findAlias(ctx, &state)
	if err != nil {
		if errors.Is(err, multipasscli.ErrNotFound) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to list aliases", err.Error())
		return
	}
	state.Context = types.StringValue(alias.Context)
	state.ID = types.StringValue(alias.Context + "." + alias.Name)
	state.Instance = types.StringValue(alias.Instance)
	// command and working_directory are kept from state because
	// the API returns the wrapped command which doesn't match
	// the user's original values.
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
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
	current, err := r.client.CurrentAliasContext(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to read alias context", err.Error())
		return
	}
	if !hasStringValue(plan.Context) || current != plan.Context.ValueString() {
		resp.Diagnostics.AddError("Alias context differs", fmt.Sprintf("Refresh state and select the recorded context %q with multipass prefer before updating this alias.", plan.Context.ValueString()))
		return
	}
	if err := r.client.DeleteAlias(ctx, plan.Context.ValueString()+"."+plan.Name.ValueString()); err != nil && !errors.Is(err, multipasscli.ErrNotFound) {
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

	alias, err := r.findAlias(ctx, &state)
	if errors.Is(err, multipasscli.ErrNotFound) {
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Failed to resolve alias", err.Error())
		return
	}
	if err := r.client.DeleteAlias(ctx, alias.Context+"."+alias.Name); err != nil && !errors.Is(err, multipasscli.ErrNotFound) {
		resp.Diagnostics.AddError("Failed to delete alias", err.Error())
	}
}

func (r *aliasResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if aliasContext, name, ok := strings.Cut(req.ID, "."); ok {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("context"), aliasContext)...)
		req.ID = name
	}
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

func (r *aliasResource) findAlias(ctx context.Context, state *aliasResourceModel) (models.Alias, error) {
	aliases, err := r.client.ListAliases(ctx, true)
	if err != nil {
		return models.Alias{}, err
	}
	var found *models.Alias
	for _, alias := range aliases {
		if alias.Name != state.Name.ValueString() {
			continue
		}
		if hasStringValue(state.Context) {
			if alias.Context != state.Context.ValueString() {
				continue
			}
		} else if hasStringValue(state.Instance) && alias.Instance != state.Instance.ValueString() {
			continue
		}
		if found != nil {
			return models.Alias{}, fmt.Errorf("alias %q exists in multiple contexts; import it using <context>.<name> to select its owner", alias.Name)
		}
		copy := alias
		found = &copy
	}
	if found == nil {
		return models.Alias{}, multipasscli.ErrNotFound
	}
	return *found, nil
}

func aliasFromModel(m *aliasResourceModel) models.Alias {
	return models.Alias{
		Name:             m.Name.ValueString(),
		Context:          m.Context.ValueString(),
		Instance:         m.Instance.ValueString(),
		Command:          m.Command.ValueString(),
		WorkingDirectory: valueOrEmpty(m.WorkingDirectory),
	}
}
