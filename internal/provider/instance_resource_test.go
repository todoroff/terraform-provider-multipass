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
