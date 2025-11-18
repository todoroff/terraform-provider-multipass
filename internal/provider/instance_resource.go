package provider

import (
	"context"
	"regexp"
	"strings"
	"time"

	stringvalidator "github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/todoroff/terraform-provider-multipass/internal/models"
	"github.com/todoroff/terraform-provider-multipass/internal/multipasscli"
)

// Ensure implementation satisfies interfaces.
var (
	_ resource.Resource                = (*instanceResource)(nil)
	_ resource.ResourceWithConfigure   = (*instanceResource)(nil)
	_ resource.ResourceWithImportState = (*instanceResource)(nil)
)

// NewInstanceResource registers the resource with the provider.
func NewInstanceResource() resource.Resource {
	return &instanceResource{}
}

type instanceResource struct {
	client       multipasscli.Client
	defaultImage string
}

func (r *instanceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_instance"
}

func (r *instanceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages Canonical Multipass instances.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Required:            true,
				Description:         "Instance name.",
				MarkdownDescription: "Instance name. Must be unique per Multipass host.",
			},
			"image": schema.StringAttribute{
				Optional:            true,
				Description:         "Image alias or name (e.g., `lts`, `jammy`, `24.04`). Defaults to provider `default_image`.",
				MarkdownDescription: "Image alias or name (e.g., `lts`, `jammy`, `24.04`). Defaults to provider `default_image`.",
			},
			"cpus": schema.Int64Attribute{
				Optional:            true,
				Description:         "Number of virtual CPUs. Changing this value forces recreation.",
				MarkdownDescription: "Number of virtual CPUs. Changing this value forces recreation.",
				PlanModifiers: []planmodifier.Int64{
					int64planmodifier.RequiresReplace(),
				},
			},
			"memory": schema.StringAttribute{
				Optional:            true,
				Description:         "Memory size (e.g., `1G`, `512M`). Changing forces recreation.",
				MarkdownDescription: "Memory size (e.g., `1G`, `512M`). Changing forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(memoryRegex, "must follow Multipass size notation, e.g. 1G or 512M"),
				},
			},
			"disk": schema.StringAttribute{
				Optional:            true,
				Description:         "Disk size (e.g., `5G`). Changing forces recreation.",
				MarkdownDescription: "Disk size (e.g., `5G`). Changing forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
				Validators: []validator.String{
					stringvalidator.RegexMatches(memoryRegex, "must follow Multipass size notation, e.g. 5G"),
				},
			},
			"cloud_init_file": schema.StringAttribute{
				Optional:            true,
				Description:         "Path to a cloud-init YAML file applied at launch. Mutually exclusive with `cloud_init`. Forces recreation.",
				MarkdownDescription: "Path to a cloud-init YAML file applied at launch. Mutually exclusive with `cloud_init`. Forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"cloud_init": schema.StringAttribute{
				Optional:            true,
				Sensitive:           true,
				Description:         "Inline cloud-init YAML applied at launch. Mutually exclusive with `cloud_init_file`. Forces recreation.",
				MarkdownDescription: "Inline cloud-init YAML applied at launch. Mutually exclusive with `cloud_init_file`. Forces recreation.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"primary": schema.BoolAttribute{
				Optional:            true,
				Description:         "If true, mark this instance as the Multipass primary instance after creation.",
				MarkdownDescription: "If true, mark this instance as the Multipass primary instance after creation.",
			},
			"auto_recover": schema.BoolAttribute{
				Optional:            true,
				Description:         "Attempt to recover the instance if it becomes deleted outside Terraform.",
				MarkdownDescription: "Attempt to recover the instance if it becomes deleted outside Terraform.",
			},
			"auto_start_on_recover": schema.BoolAttribute{
				Optional:            true,
				Description:         "If true, automatically start the instance after a successful auto-recover when it was soft-deleted outside Terraform.",
				MarkdownDescription: "If true, automatically start the instance after a successful auto-recover when it was soft-deleted outside Terraform.",
			},
			"ipv4": schema.ListAttribute{
				Computed:            true,
				Description:         "Assigned IPv4 addresses.",
				MarkdownDescription: "Assigned IPv4 addresses as reported by `multipass info`.",
				ElementType:         types.StringType,
			},
			"state": schema.StringAttribute{
				Computed:            true,
				Description:         "Current power state.",
				MarkdownDescription: "Current power state reported by Multipass.",
			},
			"release": schema.StringAttribute{
				Computed:            true,
				Description:         "Operating system release running in the instance.",
				MarkdownDescription: "Operating system release running in the instance.",
			},
			"image_release": schema.StringAttribute{
				Computed:            true,
				Description:         "Release name pulled from the image metadata.",
				MarkdownDescription: "Release name pulled from the image metadata.",
			},
			"snapshot_count": schema.Int64Attribute{
				Computed:            true,
				Description:         "Number of snapshots recorded for this instance.",
				MarkdownDescription: "Number of snapshots recorded for this instance.",
			},
			"last_updated": schema.StringAttribute{
				Computed:            true,
				Description:         "Timestamp of the last information refresh.",
				MarkdownDescription: "Timestamp of the last information refresh in RFC3339 format.",
			},
		},
		Blocks: map[string]schema.Block{
			"networks": schema.ListNestedBlock{
				Description: "Optional networks to attach during launch.",
				PlanModifiers: []planmodifier.List{
					listplanmodifier.RequiresReplace(),
				},
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Required:    true,
							Description: "Host network name (from `multipass networks`).",
						},
						"mode": schema.StringAttribute{
							Optional:    true,
							Description: "Attachment mode (auto/manual).",
						},
						"mac": schema.StringAttribute{
							Optional:    true,
							Description: "Explicit MAC address to assign.",
						},
					},
				},
			},
			"mounts": schema.ListNestedBlock{
				Description: "Host directory mounts to attach at launch.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"host_path": schema.StringAttribute{
							Required: true,
						},
						"instance_path": schema.StringAttribute{
							Required: true,
						},
						"read_only": schema.BoolAttribute{
							Optional: true,
						},
					},
				},
			},
		},
	}
}

func (r *instanceResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	data := req.ProviderData.(providerData)
	r.client = data.client
	r.defaultImage = data.defaultImage
}

func (r *instanceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError("Client not configured", "The provider Multipass client was not configured.")
		return
	}

	var plan instanceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if hasStringValue(plan.CloudInitFile) && hasStringValue(plan.CloudInit) {
		resp.Diagnostics.AddAttributeError(
			path.Root("cloud_init"),
			"Conflicting cloud-init configuration",
			"Only one of cloud_init or cloud_init_file can be set. Remove one of the attributes and try again.",
		)
		return
	}

	opts := models.LaunchOptions{
		Name:            plan.Name.ValueString(),
		Image:           r.resolveImage(plan.Image),
		CPUs:            valueOrDefaultInt(plan.CPUs, 1),
		Memory:          valueOrDefaultString(plan.Memory, "1G"),
		Disk:            valueOrDefaultString(plan.Disk, "5G"),
		CloudInitFile:   valueOrEmpty(plan.CloudInitFile),
		CloudInitInline: valueOrEmpty(plan.CloudInit),
		Networks:        expandNetworkAttachments(plan.Networks),
		Mounts:          expandMounts(plan.Mounts),
		Primary:         plan.Primary.ValueBool(),
	}

	if err := r.client.LaunchInstance(ctx, opts); err != nil {
		resp.Diagnostics.AddError("Failed to launch instance", err.Error())
		return
	}

	if opts.Primary {
		if err := r.client.SetPrimary(ctx, opts.Name); err != nil {
			resp.Diagnostics.AddWarning("Failed to set primary", err.Error())
		}
	}

	diags := r.refreshState(ctx, opts.Name, &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *instanceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError("Client not configured", "The provider Multipass client was not configured.")
		return
	}

	var state instanceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := state.Name.ValueString()
	instance, err := r.client.GetInstance(ctx, name)

	// If the instance is missing and auto_recover is enabled, attempt a recover.
	if err == multipasscli.ErrNotFound && state.AutoRecover.ValueBool() {
		if recErr := r.client.RecoverInstance(ctx, name); recErr != nil {
			resp.Diagnostics.AddWarning("Failed to auto-recover instance", recErr.Error())
			resp.State.RemoveResource(ctx)
			return
		}
		instance, err = r.client.GetInstance(ctx, name)

		// Optionally start the instance after a successful recover.
		if err == nil && state.AutoStartOnRecover.ValueBool() && !strings.EqualFold(instance.State, "Running") {
			if startErr := r.client.StartInstance(ctx, name); startErr != nil {
				resp.Diagnostics.AddWarning("Failed to auto-start instance after recover", startErr.Error())
			} else {
				instance, err = r.client.GetInstance(ctx, name)
			}
		}
	}

	if err != nil {
		if err == multipasscli.ErrNotFound {
			tflog.Info(ctx, "Multipass instance no longer exists", map[string]any{"name": name})
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Failed to read instance", err.Error())
		return
	}

	// Multipass keeps "soft-deleted" instances around with a Deleted state that can be
	// recovered via `multipass recover`. If auto_recover is enabled, transparently
	// recover such instances so Terraform can continue managing them.
	if state.AutoRecover.ValueBool() && strings.EqualFold(instance.State, "Deleted") {
		if recErr := r.client.RecoverInstance(ctx, name); recErr != nil {
			resp.Diagnostics.AddWarning("Failed to auto-recover soft-deleted instance", recErr.Error())
		} else {
			instance, err = r.client.GetInstance(ctx, name)
			if err != nil {
				resp.Diagnostics.AddError("Failed to read instance after auto-recover", err.Error())
				return
			}

			// Optionally start the instance after a successful recover from Deleted state.
			if state.AutoStartOnRecover.ValueBool() && !strings.EqualFold(instance.State, "Running") {
				if startErr := r.client.StartInstance(ctx, name); startErr != nil {
					resp.Diagnostics.AddWarning("Failed to auto-start instance after recover", startErr.Error())
				} else {
					instance, err = r.client.GetInstance(ctx, name)
					if err != nil {
						resp.Diagnostics.AddError("Failed to read instance after auto-start", err.Error())
						return
					}
				}
			}
		}
	}

	resp.Diagnostics.Append(applyInstanceToModel(ctx, instance, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *instanceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError("Client not configured", "The provider Multipass client was not configured.")
		return
	}

	var plan instanceResourceModel
	var state instanceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	if hasStringValue(plan.CloudInitFile) && hasStringValue(plan.CloudInit) {
		resp.Diagnostics.AddAttributeError(
			path.Root("cloud_init"),
			"Conflicting cloud-init configuration",
			"Only one of cloud_init or cloud_init_file can be set. Remove one of the attributes and try again.",
		)
		return
	}

	if plan.Primary.ValueBool() && !state.Primary.ValueBool() {
		if err := r.client.SetPrimary(ctx, plan.Name.ValueString()); err != nil {
			resp.Diagnostics.AddError("Failed to set primary", err.Error())
			return
		}
	}

	toAdd, toRemove := diffMounts(plan.Mounts, state.Mounts)
	if len(toAdd) > 0 || len(toRemove) > 0 {
		// Simplify lifecycle: unmount all current mounts, then recreate the
		// desired set from the plan. This avoids depending on per-path umount
		// semantics and guarantees the final set matches Terraform config.
		if err := r.client.Unmount(ctx, plan.Name.ValueString(), models.Mount{}); err != nil {
			resp.Diagnostics.AddError("Failed to unmount existing mounts", err.Error())
			return
		}

		for _, m := range plan.Mounts {
			if err := r.client.Mount(ctx, plan.Name.ValueString(), mountConfigToModel(m)); err != nil {
				resp.Diagnostics.AddError("Failed to mount directory", err.Error())
				return
			}
		}
	}

	diags := r.refreshState(ctx, plan.Name.ValueString(), &plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *instanceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	if r.client == nil {
		resp.Diagnostics.AddError("Client not configured", "The provider Multipass client was not configured.")
		return
	}

	var state instanceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	name := state.Name.ValueString()
	if err := r.client.DeleteInstance(ctx, name, true); err != nil {
		if err == multipasscli.ErrNotFound {
			return
		}
		resp.Diagnostics.AddError("Failed to delete instance", err.Error())
	}
}

func (r *instanceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("name"), req, resp)
}

func (r *instanceResource) refreshState(ctx context.Context, name string, model *instanceResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	instance, err := r.client.GetInstance(ctx, name)
	if err != nil {
		diags.AddError("Failed to refresh instance state", err.Error())
		return diags
	}

	model.ID = types.StringValue(name)
	model.Name = types.StringValue(name)
	diags.Append(applyInstanceToModel(ctx, instance, model)...)
	return diags
}

func applyInstanceToModel(ctx context.Context, instance *models.Instance, model *instanceResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics

	model.State = types.StringValue(instance.State)
	model.Release = types.StringValue(instance.Release)
	model.ImageRelease = types.StringValue(instance.ImageRelease)
	model.SnapshotCount = types.Int64Value(int64(instance.SnapshotCount))
	model.LastUpdated = types.StringValue(instance.LastUpdated.UTC().Format(time.RFC3339))

	if len(instance.IPv4) > 0 {
		list, diag := types.ListValueFrom(ctx, types.StringType, instance.IPv4)
		diags.Append(diag...)
		model.IPv4 = list
	} else {
		model.IPv4 = types.ListNull(types.StringType)
	}

	return diags
}

// Helpers

var memoryRegex = regexp.MustCompile(`^[0-9]+(K|M|G|T)$`)

type networkConfigModel struct {
	Name types.String `tfsdk:"name"`
	Mode types.String `tfsdk:"mode"`
	Mac  types.String `tfsdk:"mac"`
}

type mountConfigModel struct {
	HostPath     types.String `tfsdk:"host_path"`
	InstancePath types.String `tfsdk:"instance_path"`
	ReadOnly     types.Bool   `tfsdk:"read_only"`
}

type instanceResourceModel struct {
	ID                 types.String         `tfsdk:"id"`
	Name               types.String         `tfsdk:"name"`
	Image              types.String         `tfsdk:"image"`
	CPUs               types.Int64          `tfsdk:"cpus"`
	Memory             types.String         `tfsdk:"memory"`
	Disk               types.String         `tfsdk:"disk"`
	CloudInitFile      types.String         `tfsdk:"cloud_init_file"`
	CloudInit          types.String         `tfsdk:"cloud_init"`
	Primary            types.Bool           `tfsdk:"primary"`
	AutoRecover        types.Bool           `tfsdk:"auto_recover"`
	AutoStartOnRecover types.Bool           `tfsdk:"auto_start_on_recover"`
	Networks           []networkConfigModel `tfsdk:"networks"`
	Mounts             []mountConfigModel   `tfsdk:"mounts"`
	IPv4               types.List           `tfsdk:"ipv4"`
	State              types.String         `tfsdk:"state"`
	Release            types.String         `tfsdk:"release"`
	ImageRelease       types.String         `tfsdk:"image_release"`
	SnapshotCount      types.Int64          `tfsdk:"snapshot_count"`
	LastUpdated        types.String         `tfsdk:"last_updated"`
}

func (r *instanceResource) resolveImage(image types.String) string {
	if !image.IsNull() && !image.IsUnknown() && image.ValueString() != "" {
		return image.ValueString()
	}
	if r.defaultImage != "" {
		return r.defaultImage
	}
	return "lts"
}

func expandNetworkAttachments(configs []networkConfigModel) []models.NetworkAttachment {
	result := make([]models.NetworkAttachment, 0, len(configs))
	for _, m := range configs {
		if m.Name.IsUnknown() || m.Name.IsNull() || m.Name.ValueString() == "" {
			continue
		}
		result = append(result, models.NetworkAttachment{
			Name: m.Name.ValueString(),
			Mode: valueOrEmpty(m.Mode),
			Mac:  valueOrEmpty(m.Mac),
		})
	}
	return result
}

func expandMounts(configs []mountConfigModel) []models.Mount {
	result := make([]models.Mount, 0, len(configs))
	for _, m := range configs {
		if m.HostPath.ValueString() == "" || m.InstancePath.ValueString() == "" {
			continue
		}
		result = append(result, models.Mount{
			HostPath:     m.HostPath.ValueString(),
			InstancePath: m.InstancePath.ValueString(),
			ReadOnly:     m.ReadOnly.ValueBool(),
		})
	}
	return result
}

func mountConfigToModel(m mountConfigModel) models.Mount {
	return models.Mount{
		HostPath:     m.HostPath.ValueString(),
		InstancePath: m.InstancePath.ValueString(),
		ReadOnly:     m.ReadOnly.ValueBool(),
	}
}

func diffMounts(plan, state []mountConfigModel) (toAdd, toRemove []mountConfigModel) {
	planMap := mountConfigMap(plan)
	stateMap := mountConfigMap(state)

	for key, current := range stateMap {
		desired, exists := planMap[key]
		if !exists {
			toRemove = append(toRemove, current)
			continue
		}
		if current.ReadOnly.ValueBool() != desired.ReadOnly.ValueBool() {
			toRemove = append(toRemove, current)
			toAdd = append(toAdd, desired)
		}
	}

	for key, desired := range planMap {
		if _, exists := stateMap[key]; !exists {
			toAdd = append(toAdd, desired)
		}
	}

	return
}

func mountConfigMap(configs []mountConfigModel) map[string]mountConfigModel {
	result := make(map[string]mountConfigModel, len(configs))
	for _, c := range configs {
		host := c.HostPath.ValueString()
		instance := c.InstancePath.ValueString()
		if host == "" || instance == "" {
			continue
		}
		key := host + "|" + instance
		result[key] = c
	}
	return result
}

func valueOrEmpty(v types.String) string {
	if v.IsNull() || v.IsUnknown() {
		return ""
	}
	return v.ValueString()
}

func hasStringValue(v types.String) bool {
	return !v.IsNull() && !v.IsUnknown() && v.ValueString() != ""
}

func valueOrDefaultString(v types.String, def string) string {
	if v.IsNull() || v.IsUnknown() || v.ValueString() == "" {
		return def
	}
	return v.ValueString()
}

func valueOrDefaultInt(v types.Int64, def int) int {
	if v.IsNull() || v.IsUnknown() {
		return def
	}
	return int(v.ValueInt64())
}
