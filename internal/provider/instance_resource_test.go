package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
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
