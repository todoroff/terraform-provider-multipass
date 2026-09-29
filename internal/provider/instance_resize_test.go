package provider

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/todoroff/terraform-provider-multipass/internal/models"
	"github.com/todoroff/terraform-provider-multipass/internal/multipasscli"
)

func resizeValues() map[string]any {
	return map[string]any{"id": "vm", "name": "vm", "cpus": int64(2), "memory": "2G", "disk": "10G", "resize_policy": "in_place", "state": "Running", "networks": []tftypes.Value{}, "mounts": []tftypes.Value{}}
}

func TestResizePlanning(t *testing.T) {
	for _, tc := range []struct {
		name          string
		policy        any
		changes       map[string]any
		priorChanges  map[string]any
		replace       bool
		errorExpected bool
	}{
		{name: "default CPU", changes: map[string]any{"cpus": 4}},
		{name: "memory decrease", policy: "in_place", changes: map[string]any{"memory": "1G"}},
		{name: "disk growth", changes: map[string]any{"disk": "12G"}},
		{name: "replacement CPU", policy: "replace", changes: map[string]any{"cpus": 4}, replace: true},
		{name: "replacement memory", policy: "replace", changes: map[string]any{"memory": "1G"}, replace: true},
		{name: "replacement disk shrink", policy: "replace", changes: map[string]any{"disk": "5G"}, replace: true},
		{name: "policy only", policy: "replace"},
		{name: "switch to in place", policy: "in_place", priorChanges: map[string]any{"resize_policy": "replace"}, changes: map[string]any{"cpus": 4}},
		{name: "equivalent sizes", policy: "replace", changes: map[string]any{"memory": "2048M", "disk": "10240M"}},
		{name: "removed sizes", policy: "replace", changes: map[string]any{"cpus": nil, "memory": nil, "disk": nil}},
		{name: "legacy null sizes", policy: "replace", priorChanges: map[string]any{"cpus": nil, "memory": nil, "disk": nil, "resize_policy": nil}, changes: map[string]any{"cpus": nil, "memory": nil, "disk": nil}},
		{name: "unknown CPU in place", changes: map[string]any{"cpus": tftypes.UnknownValue}},
		{name: "unknown CPU replace", policy: "replace", changes: map[string]any{"cpus": tftypes.UnknownValue}, replace: true},
		{name: "unknown policy and size", policy: tftypes.UnknownValue, changes: map[string]any{"cpus": 4}, errorExpected: true},
		{name: "unknown policy only", policy: tftypes.UnknownValue},
		{name: "name still replaces", changes: map[string]any{"name": "another"}, replace: true},
		{name: "image still replaces", changes: map[string]any{"image": "24.04"}, replace: true},
		{name: "cloud init still replaces", changes: map[string]any{"cloud_init": "#cloud-config"}, replace: true},
		{name: "cloud init file still replaces", changes: map[string]any{"cloud_init_file": "cloud.yaml"}, replace: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			s := resourceSchema(NewInstanceResource())
			prior := resizeValues()
			for k, v := range tc.priorChanges {
				prior[k] = v
			}
			stateValue := resourceValue(s, prior)
			next := resizeValues()
			next["resize_policy"] = tc.policy
			for k, v := range tc.changes {
				next[k] = v
			}
			proposed := resourceValue(s, next)
			delete(next, "id")
			delete(next, "state")
			server := providerserver.NewProtocol6(New("test")())()
			resp, err := server.PlanResourceChange(ctx, &tfprotov6.PlanResourceChangeRequest{TypeName: "multipass_instance", PriorState: dynamicValue(t, stateValue), ProposedNewState: dynamicValue(t, proposed), Config: dynamicValue(t, resourceValue(s, next))})
			if err != nil {
				t.Fatal(err)
			}
			hasError := false
			for _, d := range resp.Diagnostics {
				hasError = hasError || d.Severity == tfprotov6.DiagnosticSeverityError
			}
			if hasError != tc.errorExpected {
				t.Fatalf("diagnostics: %v", resp.Diagnostics)
			}
			if (len(resp.RequiresReplace) > 0) != tc.replace {
				t.Fatalf("replacement paths %v; want replace=%v", resp.RequiresReplace, tc.replace)
			}
			if hasError {
				return
			}
			planned, err := resp.PlannedState.Unmarshal(s.Type().TerraformType(ctx))
			if err != nil {
				t.Fatal(err)
			}
			var attrs map[string]tftypes.Value
			if err := planned.As(&attrs); err != nil {
				t.Fatal(err)
			}
			if tc.policy == nil {
				var p string
				if err := attrs["resize_policy"].As(&p); err != nil || p != "in_place" {
					t.Fatalf("default policy: %s, %v", p, err)
				}
			}
			if strings.HasPrefix(tc.name, "unknown CPU") && attrs["cpus"].IsKnown() {
				t.Fatal("unknown configuration was overwritten")
			}
			if tc.name == "removed sizes" {
				for _, k := range []string{"cpus", "memory", "disk"} {
					if !attrs[k].Equal(stateValueAttribute(t, stateValue, k)) {
						t.Fatalf("removed %s lost allocation", k)
					}
				}
			}
		})
	}
}

func stateValueAttribute(t *testing.T, value tftypes.Value, key string) tftypes.Value {
	t.Helper()
	var attrs map[string]tftypes.Value
	if err := value.As(&attrs); err != nil {
		t.Fatal(err)
	}
	return attrs[key]
}

type resizeFixture struct {
	alloc          models.InstanceResources
	power          string
	commands       []string
	fail           string
	readFail       bool
	stopDelay      bool
	stopTookEffect bool
	client         *testClient
}

func newResizeFixture() *resizeFixture {
	f := &resizeFixture{alloc: models.InstanceResources{CPUs: 2, MemoryBytes: 2 << 30, DiskBytes: 10 << 30}, power: "Running"}
	f.client = &testClient{
		getResources: func(ctx context.Context, _ string) (models.InstanceResources, error) {
			if err := ctx.Err(); err != nil {
				return models.InstanceResources{}, err
			}
			if f.readFail && len(f.commands) > 0 {
				return models.InstanceResources{}, errors.New("readback failed")
			}
			return f.alloc, nil
		},
		listInstances: func(ctx context.Context, refresh bool) ([]models.Instance, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if !refresh {
				panic("power state must bypass cache")
			}
			return []models.Instance{{Name: "vm", State: f.power, LastUpdated: time.Now()}}, nil
		},
		getInstance: func(ctx context.Context, name string) (*models.Instance, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return &models.Instance{Name: name, State: f.power, LastUpdated: time.Now()}, nil
		},
		stopInstance: func(ctx context.Context, _ string, force bool) error {
			if force {
				panic("resize must use graceful stop")
			}
			f.commands = append(f.commands, "stop")
			if f.stopDelay {
				f.power = "Stopping"
				<-ctx.Done()
				return ctx.Err()
			}
			if f.fail == "stop" {
				if f.stopTookEffect {
					f.power = "Stopped"
				}
				return errors.New("stop failed")
			}
			f.power = "Stopped"
			return nil
		},
		startInstance: func(_ context.Context, _ string) error {
			f.commands = append(f.commands, "start")
			if f.fail == "start" {
				return errors.New("start failed")
			}
			f.power = "Running"
			return nil
		},
		setResource: func(_ context.Context, _ string, setting multipasscli.ResourceSetting, value uint64) error {
			if f.power != "Stopped" {
				panic("set called before stopped")
			}
			f.commands = append(f.commands, string(setting))
			if f.fail == string(setting) {
				return fmt.Errorf("%s failed", setting)
			}
			switch setting {
			case multipasscli.ResourceCPUs:
				f.alloc.CPUs = int64(value)
			case multipasscli.ResourceMemory:
				f.alloc.MemoryBytes = value
			case multipasscli.ResourceDisk:
				f.alloc.DiskBytes = value
			}
			return nil
		},
	}
	return f
}

func updateWithFixture(t *testing.T, ctx context.Context, f *resizeFixture, changes map[string]any) (instanceResourceModel, resource.UpdateResponse) {
	t.Helper()
	r := &instanceResource{client: f.client, commandTimeout: 100 * time.Millisecond}
	s := resourceSchema(r)
	values := resizeValues()
	values["state"] = f.power
	state := tfsdk.State{Schema: s, Raw: resourceValue(s, values)}
	for k, v := range changes {
		values[k] = v
	}
	resp := resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{State: state, Plan: tfsdk.Plan{Schema: s, Raw: resourceValue(s, values)}}, &resp)
	var result instanceResourceModel
	if d := resp.State.Get(context.Background(), &result); d.HasError() {
		t.Fatal(d)
	}
	return result, resp
}

func TestResizeLifecycle(t *testing.T) {
	for _, tc := range []struct {
		name, power   string
		changes       map[string]any
		commands      []string
		errorExpected bool
	}{
		{"running", "Running", map[string]any{"cpus": 4, "memory": "3G", "disk": "12G"}, []string{"stop", "cpus", "memory", "disk", "start"}, false},
		{"stopped", "Stopped", map[string]any{"cpus": 1, "memory": "1G"}, []string{"cpus", "memory"}, false},
		{"equivalent", "Running", map[string]any{"memory": "2048M", "disk": "10240M"}, nil, false},
		{"policy only", "Running", map[string]any{"resize_policy": "replace"}, nil, false},
		{"omitted", "Running", map[string]any{"cpus": nil, "memory": nil, "disk": nil}, nil, false},
		{"shrink", "Running", map[string]any{"cpus": 4, "disk": "5G"}, nil, true},
		{"suspended", "Suspended", map[string]any{"cpus": 4}, nil, true},
		{"transitional", "Starting", map[string]any{"cpus": 4}, nil, true},
		{"deleted", "Deleted", map[string]any{"cpus": 4}, nil, true},
		{"replace cannot resize during update", "Running", map[string]any{"cpus": 4, "resize_policy": "replace"}, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newResizeFixture()
			f.power = tc.power
			state, resp := updateWithFixture(t, context.Background(), f, tc.changes)
			if resp.Diagnostics.HasError() != tc.errorExpected {
				t.Fatalf("diagnostics: %v", resp.Diagnostics)
			}
			if !reflect.DeepEqual(f.commands, tc.commands) {
				t.Fatalf("commands %v, want %v", f.commands, tc.commands)
			}
			if f.power != tc.power {
				t.Fatalf("power %s, want %s", f.power, tc.power)
			}
			if state.CPUs.ValueInt64() != f.alloc.CPUs {
				t.Fatalf("state CPUs %v, actual %d", state.CPUs, f.alloc.CPUs)
			}
			if tc.name == "equivalent" && state.Memory.ValueString() != "2048M" {
				t.Fatal("plan spelling was not preserved")
			}
		})
	}
}

func TestResizePartialFailures(t *testing.T) {
	for _, failure := range []string{"stop", "cpus", "memory", "disk", "start", "readback"} {
		t.Run(failure, func(t *testing.T) {
			f := newResizeFixture()
			f.fail = failure
			f.readFail = failure == "readback"
			state, resp := updateWithFixture(t, context.Background(), f, map[string]any{"cpus": 4, "memory": "3G", "disk": "12G", "resize_policy": "in_place", "auto_recover": true})
			if !resp.Diagnostics.HasError() {
				t.Fatal("expected failure")
			}
			if state.CPUs.ValueInt64() != f.alloc.CPUs {
				t.Fatalf("partial CPU change lost: %+v", state)
			}
			memory, err := multipasscli.ParseSize(state.Memory.ValueString())
			if err != nil || memory != f.alloc.MemoryBytes {
				t.Fatalf("partial memory change lost: %v", state.Memory)
			}
			disk, err := multipasscli.ParseSize(state.Disk.ValueString())
			if err != nil || disk != f.alloc.DiskBytes {
				t.Fatalf("partial disk change lost: %v", state.Disk)
			}
			if state.AutoRecover.ValueBool() {
				t.Fatal("unapplied plan configuration leaked into failed state")
			}
			if failure != "start" && f.power != "Running" {
				t.Fatalf("failed resize did not restore running state: %s", f.power)
			}
			if failure == "memory" {
				if !reflect.DeepEqual(f.commands, []string{"stop", "cpus", "memory", "start"}) {
					t.Fatalf("continued after failure: %v", f.commands)
				}
			}
		})
	}
}

func TestResizeRetrySkipsAlreadyAppliedAllocations(t *testing.T) {
	f := newResizeFixture()
	f.fail = "memory"
	changes := map[string]any{"cpus": 4, "memory": "3G", "disk": "12G"}
	_, first := updateWithFixture(t, context.Background(), f, changes)
	if !first.Diagnostics.HasError() {
		t.Fatal("expected initial failure")
	}
	f.fail = ""
	f.commands = nil
	r := &instanceResource{client: f.client, commandTimeout: time.Second}
	s := resourceSchema(r)
	values := resizeValues()
	for k, v := range changes {
		values[k] = v
	}
	resp := resource.UpdateResponse{State: first.State}
	r.Update(context.Background(), resource.UpdateRequest{State: first.State, Plan: tfsdk.Plan{Schema: s, Raw: resourceValue(s, values)}}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if !reflect.DeepEqual(f.commands, []string{"stop", "memory", "disk", "start"}) {
		t.Fatalf("retry commands: %v", f.commands)
	}
}

func TestResizeStoppedFailureStaysStopped(t *testing.T) {
	f := newResizeFixture()
	f.power = "Stopped"
	f.fail = "memory"
	state, resp := updateWithFixture(t, context.Background(), f, map[string]any{"cpus": 4, "memory": "3G"})
	if !resp.Diagnostics.HasError() || f.power != "Stopped" || state.State.ValueString() != "Stopped" {
		t.Fatalf("power state after failure: %s, %v", f.power, resp.Diagnostics)
	}
	if !reflect.DeepEqual(f.commands, []string{"cpus", "memory"}) {
		t.Fatalf("unexpected lifecycle command: %v", f.commands)
	}
}

func TestResizeReadbackMismatchPersistsReality(t *testing.T) {
	f := newResizeFixture()
	setter := f.client.setResource
	f.client.setResource = func(ctx context.Context, name string, setting multipasscli.ResourceSetting, value uint64) error {
		if err := setter(ctx, name, setting, value); err != nil {
			return err
		}
		if setting == multipasscli.ResourceMemory {
			f.alloc.MemoryBytes = 4 << 30
		}
		return nil
	}
	state, resp := updateWithFixture(t, context.Background(), f, map[string]any{"memory": "3G"})
	if !resp.Diagnostics.HasError() || state.Memory.ValueString() != "4294967296" || f.power != "Running" {
		t.Fatalf("verification failure was not reconciled: %v, %v, %s", resp.Diagnostics, state.Memory, f.power)
	}
}

func TestResizePreservesExactRequestWithRoundedReadback(t *testing.T) {
	f := newResizeFixture()
	getter := f.client.getResources
	f.client.getResources = func(ctx context.Context, name string) (models.InstanceResources, error) {
		actual, err := getter(ctx, name)
		if actual.MemoryBytes == 1537<<20 {
			actual.MemoryBytes = 1536 << 20
			actual.MemoryDisplay = "1.5GiB"
		}
		return actual, err
	}
	state, resp := updateWithFixture(t, context.Background(), f, map[string]any{"memory": "1537M"})
	if resp.Diagnostics.HasError() || state.Memory.ValueString() != "1537M" {
		t.Fatalf("rounded readback changed request: %v, %v", state.Memory, resp.Diagnostics)
	}
	f.commands = nil
	r := &instanceResource{client: f.client, commandTimeout: time.Second}
	readResp := resource.ReadResponse{State: resp.State}
	r.Read(context.Background(), resource.ReadRequest{State: resp.State}, &readResp)
	if readResp.Diagnostics.HasError() {
		t.Fatal(readResp.Diagnostics)
	}
	if d := readResp.State.Get(context.Background(), &state); d.HasError() {
		t.Fatal(d)
	}
	if state.Memory.ValueString() != "1537M" || len(f.commands) > 0 {
		t.Fatal("refresh lost precise configured allocation")
	}
}

func TestAllocationSchemaValidation(t *testing.T) {
	for _, tc := range []struct {
		field string
		value any
		valid bool
	}{
		{"resize_policy", "in_place", true}, {"resize_policy", "replace", true}, {"resize_policy", "auto", false},
		{"cpus", 0, false}, {"cpus", -1, false}, {"cpus", 1, true},
		{"memory", "0G", false}, {"memory", "1.5GiB", true}, {"disk", "1024", true}, {"disk", "999999999999999999999T", false},
	} {
		t.Run(fmt.Sprintf("%s=%v", tc.field, tc.value), func(t *testing.T) {
			s := resourceSchema(NewInstanceResource())
			config := resizeValues()
			delete(config, "id")
			delete(config, "state")
			config[tc.field] = tc.value
			server := providerserver.NewProtocol6(New("test")())()
			resp, err := server.ValidateResourceConfig(context.Background(), &tfprotov6.ValidateResourceConfigRequest{TypeName: "multipass_instance", Config: dynamicValue(t, resourceValue(s, config))})
			if err != nil {
				t.Fatal(err)
			}
			hasError := false
			for _, d := range resp.Diagnostics {
				hasError = hasError || d.Severity == tfprotov6.DiagnosticSeverityError
			}
			if hasError == tc.valid {
				t.Fatalf("validation: %v", resp.Diagnostics)
			}
		})
	}
}

func TestResizeRecoversStopThatFailedAfterTakingEffect(t *testing.T) {
	f := newResizeFixture()
	f.fail = "stop"
	f.stopTookEffect = true
	_, resp := updateWithFixture(t, context.Background(), f, map[string]any{"cpus": 4})
	if !resp.Diagnostics.HasError() || f.power != "Running" || !reflect.DeepEqual(f.commands, []string{"stop", "start"}) {
		t.Fatalf("recovery: %v, %s, %v", resp.Diagnostics, f.power, f.commands)
	}
}

func TestResizeTimeoutAndCancellation(t *testing.T) {
	f := newResizeFixture()
	f.stopDelay = true
	_, resp := updateWithFixture(t, context.Background(), f, map[string]any{"cpus": 4})
	if !resp.Diagnostics.HasError() || !reflect.DeepEqual(f.commands, []string{"stop"}) {
		t.Fatalf("timeout: %v, %v", resp.Diagnostics, f.commands)
	}
	f = newResizeFixture()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, resp = updateWithFixture(t, ctx, f, map[string]any{"cpus": 4})
	if !resp.Diagnostics.HasError() || len(f.commands) > 0 {
		t.Fatalf("cancelled request mutated VM: %v", f.commands)
	}
}

func TestReadRefreshesAllocationsAndPreservesUnits(t *testing.T) {
	f := newResizeFixture()
	f.alloc.CPUs = 4
	f.alloc.DiskBytes = 12 << 30
	f.power = "Stopped"
	r := &instanceResource{client: f.client, commandTimeout: time.Second}
	s := resourceSchema(r)
	values := resizeValues()
	values["resize_policy"] = nil
	state := tfsdk.State{Schema: s, Raw: resourceValue(s, values)}
	resp := resource.ReadResponse{State: state}
	r.Read(context.Background(), resource.ReadRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var got instanceResourceModel
	if d := resp.State.Get(context.Background(), &got); d.HasError() {
		t.Fatal(d)
	}
	if got.CPUs.ValueInt64() != 4 || got.Memory.ValueString() != "2G" || got.Disk.ValueString() != "12884901888" || got.ResizePolicy.ValueString() != "in_place" {
		t.Fatalf("state not refreshed: %+v", got)
	}
	if len(f.commands) != 0 {
		t.Fatal("read mutated VM")
	}
}

func TestCreatedVMRemainsManagedWhenAllocationReadFails(t *testing.T) {
	ctx := context.Background()
	r := &instanceResource{commandTimeout: time.Second, client: &testClient{
		launchInstance: func(context.Context, models.LaunchOptions) error { return nil },
		getResources: func(context.Context, string) (models.InstanceResources, error) {
			return models.InstanceResources{}, errors.New("allocation service unavailable")
		},
	}}
	s := resourceSchema(r)
	plan := tfsdk.Plan{Schema: s, Raw: resourceValue(s, map[string]any{"name": "vm", "cpus": tftypes.UnknownValue, "memory": tftypes.UnknownValue, "disk": tftypes.UnknownValue, "resize_policy": "in_place", "networks": []tftypes.Value{}, "mounts": []tftypes.Value{}})}
	resp := resource.CreateResponse{State: tfsdk.State{Schema: s}}
	r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected readback error")
	}
	var state instanceResourceModel
	if d := resp.State.Get(ctx, &state); d.HasError() {
		t.Fatal(d)
	}
	if state.ID.ValueString() != "vm" || state.CPUs.ValueInt64() != 1 || state.Memory.ValueString() != "1G" || state.Disk.ValueString() != "5G" {
		t.Fatalf("created VM lost from state: %+v", state)
	}
}
