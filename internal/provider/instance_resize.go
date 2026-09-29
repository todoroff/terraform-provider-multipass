package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/todoroff/terraform-provider-multipass/internal/models"
	"github.com/todoroff/terraform-provider-multipass/internal/multipasscli"
)

type allocationSizeValidator struct{}

func (allocationSizeValidator) Description(context.Context) string {
	return "A positive byte count or binary size such as 512M or 4G, up to 9223372036854775807 bytes."
}
func (v allocationSizeValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}
func (allocationSizeValidator) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if _, err := multipasscli.ParseSize(req.ConfigValue.ValueString()); err != nil {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid allocation size", err.Error())
	}
}

func equalSize(a, b types.String) bool {
	if a.Equal(b) {
		return true
	}
	if a.IsUnknown() || a.IsNull() || b.IsUnknown() || b.IsNull() {
		return false
	}
	av, ae := multipasscli.ParseSize(a.ValueString())
	bv, be := multipasscli.ParseSize(b.ValueString())
	return ae == nil && be == nil && av == bv
}

func (r *instanceResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	var plan, state, config instanceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var changes path.Paths
	// Unconfigured values are adopted from the daemon, including legacy null
	// state. They must not trigger replacement when first discovered.
	if !config.CPUs.IsNull() && !plan.CPUs.Equal(state.CPUs) {
		changes = append(changes, path.Root("cpus"))
	}
	if !config.Memory.IsNull() && !equalSize(plan.Memory, state.Memory) {
		changes = append(changes, path.Root("memory"))
	}
	if !config.Disk.IsNull() && !equalSize(plan.Disk, state.Disk) {
		changes = append(changes, path.Root("disk"))
	}
	if len(changes) == 0 {
		return
	}
	if plan.ResizePolicy.IsUnknown() {
		resp.Diagnostics.AddAttributeError(path.Root("resize_policy"), "Unknown resize policy", "resize_policy must be known during planning when CPU, memory, or disk allocations may change.")
		return
	}
	if plan.ResizePolicy.ValueString() == "replace" {
		resp.RequiresReplace = append(resp.RequiresReplace, changes...)
	}
}

// allocationString preserves the spelling required by Terraform's planned
// value when the daemon returns an equivalent value in a different unit.
func allocationString(previous types.String, bytes uint64, display string) types.String {
	if !previous.IsNull() && !previous.IsUnknown() {
		if size, err := multipasscli.ParseSize(previous.ValueString()); err == nil && multipasscli.SizeMatchesReport(size, bytes, display) {
			return previous
		}
	}
	return types.StringValue(strconv.FormatUint(bytes, 10))
}

func applyAllocations(model *instanceResourceModel, allocations models.InstanceResources) {
	model.CPUs = types.Int64Value(allocations.CPUs)
	model.Memory = allocationString(model.Memory, allocations.MemoryBytes, allocations.MemoryDisplay)
	model.Disk = allocationString(model.Disk, allocations.DiskBytes, allocations.DiskDisplay)
}

// Retain a known allocation rather than replacing it with a rounded estimate.
// For imported or externally changed values without a matching known value,
// the CLI report is the only available allocation estimate.
func retainKnownAllocations(actual models.InstanceResources, model *instanceResourceModel) models.InstanceResources {
	if known, err := multipasscli.ParseSize(model.Memory.ValueString()); err == nil && multipasscli.SizeMatchesReport(known, actual.MemoryBytes, actual.MemoryDisplay) {
		actual.MemoryBytes = known
	}
	if known, err := multipasscli.ParseSize(model.Disk.ValueString()); err == nil && multipasscli.SizeMatchesReport(known, actual.DiskBytes, actual.DiskDisplay) {
		actual.DiskBytes = known
	}
	return actual
}

func (r *instanceResource) refreshAllocations(ctx context.Context, name string, model *instanceResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	allocations, err := r.client.GetInstanceResources(ctx, name)
	if err != nil {
		diags.AddError("Failed to read instance allocations", err.Error())
		return diags
	}
	applyAllocations(model, allocations)
	if model.ResizePolicy.IsNull() {
		model.ResizePolicy = types.StringValue("in_place")
	}
	return diags
}

func desiredAllocations(plan *instanceResourceModel, current models.InstanceResources) (models.InstanceResources, error) {
	want := current
	if !plan.CPUs.IsNull() && !plan.CPUs.IsUnknown() {
		want.CPUs = plan.CPUs.ValueInt64()
	}
	if want.CPUs < 1 {
		return want, fmt.Errorf("CPU count must be positive")
	}
	for _, field := range []struct {
		value  types.String
		target *uint64
	}{{plan.Memory, &want.MemoryBytes}, {plan.Disk, &want.DiskBytes}} {
		if !field.value.IsNull() && !field.value.IsUnknown() {
			size, err := multipasscli.ParseSize(field.value.ValueString())
			if err != nil {
				return want, err
			}
			*field.target = size
		}
	}
	return want, nil
}

func (r *instanceResource) resizeInstance(requestCtx, ctx context.Context, plan, state *instanceResourceModel) (diags diag.Diagnostics) {
	name := plan.Name.ValueString()
	current, err := r.client.GetInstanceResources(ctx, name)
	if err != nil {
		diags.AddError("Failed to read instance allocations", err.Error())
		return
	}
	current = retainKnownAllocations(current, state)
	applyAllocations(state, current)
	want, err := desiredAllocations(plan, current)
	if err != nil {
		diags.AddError("Invalid instance allocations", err.Error())
		return
	}
	if current.EqualValues(want) {
		applyAllocations(plan, current)
		return
	}
	if plan.ResizePolicy.ValueString() == "replace" {
		diags.AddError("Instance replacement required", "Allocations changed since planning. Run Terraform plan again with resize_policy = \"replace\" to replace this instance.")
		return
	}
	if plan.ResizePolicy.IsUnknown() || (plan.ResizePolicy.ValueString() != "in_place" && !plan.ResizePolicy.IsNull()) {
		diags.AddError("Invalid resize policy", "resize_policy must be in_place or replace before resizing.")
		return
	}
	if want.DiskBytes < current.DiskBytes {
		diags.AddAttributeError(path.Root("disk"), "Disk shrink is not supported", fmt.Sprintf("Instance %q has %d bytes allocated; requested %d bytes. Use resize_policy = \"replace\" or an explicit Terraform replacement to create a smaller disk. Replacement deletes the existing VM and its data.", name, current.DiskBytes, want.DiskBytes))
		return
	}
	instance, err := r.getInstanceFromList(ctx, name)
	if err != nil {
		diags.AddError("Failed to read instance power state", err.Error())
		return
	}
	state.State = types.StringValue(instance.State)
	wasRunning := strings.EqualFold(instance.State, "Running")
	if !wasRunning && !strings.EqualFold(instance.State, "Stopped") {
		diags.AddError("Instance cannot be resized in its current state", fmt.Sprintf("Instance %q is %s. Start or stop it before resizing; only Running and Stopped instances are supported.", name, instance.State))
		return
	}
	// A command can take effect even if its client times out. Always read back
	// after an attempted mutation, using a bounded recovery context that still
	// respects cancellation of the original Terraform request.
	defer func() {
		if !diags.HasError() {
			return
		}
		recoveryCtx, cancel := context.WithTimeout(requestCtx, 60*time.Second)
		defer cancel()
		if recoveryCtx.Err() != nil {
			diags.AddWarning("Resize recovery cancelled", "The request was cancelled. Confirm the VM's power state and allocations before retrying.")
			return
		}
		instance, readErr := r.getInstanceFromList(recoveryCtx, name)
		if readErr != nil {
			diags.AddWarning("Failed to read power state after resize failure", readErr.Error())
		} else {
			state.State = types.StringValue(instance.State)
			if wasRunning && strings.EqualFold(instance.State, "Stopped") {
				if startErr := r.client.StartInstance(recoveryCtx, name); startErr != nil {
					diags.AddWarning("Failed to restart instance after resize failure", startErr.Error())
				} else if err := r.waitForPowerState(recoveryCtx, name, "Running"); err != nil {
					diags.AddWarning("Failed to confirm restart after resize failure", err.Error())
				} else {
					state.State = types.StringValue("Running")
				}
			}
		}
		actual, readErr := r.client.GetInstanceResources(recoveryCtx, name)
		if readErr != nil {
			diags.AddWarning("Failed to read allocations after resize failure", readErr.Error()+" Confirm allocations with multipass get before retrying.")
		} else {
			applyAllocations(state, actual)
		}
	}()
	if wasRunning {
		if err := r.client.StopInstance(ctx, name, false); err != nil {
			diags.AddError("Failed to stop instance for resizing", err.Error())
			return
		}
		if err := r.waitForPowerState(ctx, name, "Stopped"); err != nil {
			diags.AddError("Instance did not stop for resizing", err.Error())
			return
		}
		state.State = types.StringValue("Stopped")
	}
	for _, change := range []struct {
		setting       multipasscli.ResourceSetting
		before, after uint64
	}{{multipasscli.ResourceCPUs, uint64(current.CPUs), uint64(want.CPUs)}, {multipasscli.ResourceMemory, current.MemoryBytes, want.MemoryBytes}, {multipasscli.ResourceDisk, current.DiskBytes, want.DiskBytes}} {
		if change.before == change.after {
			continue
		}
		if err := r.client.SetInstanceResource(ctx, name, change.setting, change.after); err != nil {
			diags.AddError("Failed to resize instance", fmt.Sprintf("Updating %s on %q: %v", change.setting, name, err))
			return
		}
		// Retain successful commands even if later readback is unavailable.
		switch change.setting {
		case multipasscli.ResourceCPUs:
			current.CPUs = int64(change.after)
		case multipasscli.ResourceMemory:
			current.MemoryBytes = change.after
			current.MemoryDisplay = ""
		case multipasscli.ResourceDisk:
			current.DiskBytes = change.after
			current.DiskDisplay = ""
		}
		applyAllocations(state, current)
	}
	actual, err := r.client.GetInstanceResources(ctx, name)
	if err != nil {
		diags.AddError("Failed to verify resized allocations", err.Error())
		return
	}
	applyAllocations(state, actual)
	actual = retainKnownAllocations(actual, state)
	if !actual.EqualValues(want) {
		diags.AddError("Instance allocations did not match the request", fmt.Sprintf("Requested %d CPUs, %d bytes memory, and %d bytes disk; Multipass reports %d CPUs, %d bytes memory, and %d bytes disk. Run plan again to reconcile the allocations.", want.CPUs, want.MemoryBytes, want.DiskBytes, actual.CPUs, actual.MemoryBytes, actual.DiskBytes))
		return
	}
	if wasRunning {
		if err := r.client.StartInstance(ctx, name); err != nil {
			diags.AddError("Failed to restart resized instance", err.Error())
			return
		}
		if err := r.waitForPowerState(ctx, name, "Running"); err != nil {
			diags.AddError("Resized instance did not start", err.Error())
			return
		}
		state.State = types.StringValue("Running")
	}
	applyAllocations(plan, actual)
	return
}

func (r *instanceResource) waitForPowerState(ctx context.Context, name, desired string) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		instance, err := r.getInstanceFromList(ctx, name)
		if err != nil {
			return err
		}
		if strings.EqualFold(instance.State, desired) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}
