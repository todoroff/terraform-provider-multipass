package provider

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/todoroff/terraform-provider-multipass/internal/models"
)

func TestInstanceRejectsReadOnlyMountConfig(t *testing.T) {
	ctx := context.Background()
	s := resourceSchema(NewInstanceResource())
	typ := s.Type().TerraformType(ctx).(tftypes.Object)
	mountType := typ.AttributeTypes["mounts"].(tftypes.List).ElementType
	for _, readOnly := range []bool{false, true} {
		mount := tftypes.NewValue(mountType, map[string]tftypes.Value{
			"host_path":     tftypes.NewValue(tftypes.String, "/host"),
			"instance_path": tftypes.NewValue(tftypes.String, "/workspace"),
			"read_only":     tftypes.NewValue(tftypes.Bool, readOnly),
		})
		config := resourceValue(s, map[string]any{"name": "vm", "networks": []tftypes.Value{}, "mounts": []tftypes.Value{mount}})
		server := providerserver.NewProtocol6(New("test")())()
		resp, err := server.ValidateResourceConfig(ctx, &tfprotov6.ValidateResourceConfigRequest{TypeName: "multipass_instance", Config: dynamicValue(t, config)})
		if err != nil {
			t.Fatal(err)
		}
		hasError := false
		for _, d := range resp.Diagnostics {
			hasError = hasError || d.Severity == tfprotov6.DiagnosticSeverityError
		}
		if hasError != readOnly {
			t.Fatalf("read_only=%t diagnostics: %v", readOnly, resp.Diagnostics)
		}
	}
}

func TestInstanceRejectsReadOnlyMountBeforeUpdating(t *testing.T) {
	ctx := context.Background()
	// No mock mutating methods are provided: any attempted unmount or mount
	// panics, proving validation happens before changing existing mounts.
	r := &instanceResource{client: &testClient{}, commandTimeout: time.Second}
	s := resourceSchema(r)
	typ := s.Type().TerraformType(ctx).(tftypes.Object)
	mountType := typ.AttributeTypes["mounts"].(tftypes.List).ElementType
	mount := tftypes.NewValue(mountType, map[string]tftypes.Value{
		"host_path":     tftypes.NewValue(tftypes.String, "/host"),
		"instance_path": tftypes.NewValue(tftypes.String, "/workspace"),
		"read_only":     tftypes.NewValue(tftypes.Bool, true),
	})
	values := map[string]any{"id": "vm", "name": "vm", "networks": []tftypes.Value{}, "mounts": []tftypes.Value{}}
	state := tfsdk.State{Schema: s, Raw: resourceValue(s, values)}
	values["mounts"] = []tftypes.Value{mount}
	resp := resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan{Schema: s, Raw: resourceValue(s, values)}, State: state}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected read-only mount rejection")
	}
}

func TestInstanceCloudInitResult(t *testing.T) {
	for _, tc := range []struct {
		name    string
		wait    bool
		waitErr error
	}{
		{"success", true, nil}, {"failure", true, errors.New("cloud-init exited 1")}, {"timeout", true, context.DeadlineExceeded}, {"disabled", false, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			waitCalls := 0
			r := &instanceResource{commandTimeout: time.Second, client: &testClient{
				launchInstance: func(context.Context, models.LaunchOptions) error { return nil },
				exec:           func(_ context.Context, _ string, command []string) error { waitCalls++; return tc.waitErr },
			}}
			s := resourceSchema(r)
			plan := tfsdk.Plan{Schema: s, Raw: resourceValue(s, map[string]any{"name": "vm", "wait_for_cloud_init": tc.wait, "networks": []tftypes.Value{}, "mounts": []tftypes.Value{}})}
			resp := resource.CreateResponse{State: tfsdk.State{Schema: s}}
			r.Create(ctx, resource.CreateRequest{Plan: plan}, &resp)
			if resp.Diagnostics.HasError() != (tc.waitErr != nil) {
				t.Fatalf("diagnostics: %v", resp.Diagnostics)
			}
			var state instanceResourceModel
			if d := resp.State.Get(ctx, &state); d.HasError() {
				t.Fatal(d)
			}
			if state.ID.ValueString() != "vm" {
				t.Fatal("created VM must remain in state even when cloud-init fails")
			}
			if (waitCalls == 1) != tc.wait {
				t.Fatalf("wait calls=%d", waitCalls)
			}
		})
	}
}

func TestInstanceReplacementPlan(t *testing.T) {
	ctx := context.Background()
	s := resourceSchema(NewInstanceResource())
	for _, tc := range []struct {
		name    string
		field   string
		value   any
		replace bool
	}{
		{"name", "name", "renamed-vm", true},
		{"image", "image", "24.04", true},
		{"reset_image", "image", nil, true},
		{"unchanged", "image", "22.04", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := map[string]any{"id": "vm", "name": "vm", "image": "22.04", "networks": []tftypes.Value{}, "mounts": []tftypes.Value{}}
			prior := resourceValue(s, values)
			values[tc.field] = tc.value
			next := resourceValue(s, values)
			delete(values, "id")
			server := providerserver.NewProtocol6(New("test")())()
			resp, err := server.PlanResourceChange(ctx, &tfprotov6.PlanResourceChangeRequest{TypeName: "multipass_instance", PriorState: dynamicValue(t, prior), ProposedNewState: dynamicValue(t, next), Config: dynamicValue(t, resourceValue(s, values))})
			if err != nil {
				t.Fatal(err)
			}
			for _, d := range resp.Diagnostics {
				if d.Severity == tfprotov6.DiagnosticSeverityError {
					t.Fatalf("%s: %s", d.Summary, d.Detail)
				}
			}
			if (len(resp.RequiresReplace) > 0) != tc.replace {
				t.Fatalf("replacement paths: %v, want replacement=%t", resp.RequiresReplace, tc.replace)
			}
		})
	}
}
