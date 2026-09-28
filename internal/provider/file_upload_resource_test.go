package provider

import (
	"context"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestFileUploadPlanComputedInputs(t *testing.T) {
	for _, tc := range []struct {
		name, field string
		value       any
		hashUnknown bool
	}{
		{"computed_content", "content", tftypes.UnknownValue, true},
		{"computed_source", "source", tftypes.UnknownValue, true},
		{"computed_instance", "instance", tftypes.UnknownValue, false},
		{"computed_destination", "destination", tftypes.UnknownValue, false},
		{"known_content", "content", "new payload", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			r := &fileUploadResource{}
			s := resourceSchema(r)
			values := map[string]any{"instance": "vm", "destination": "/tmp/file", "content": "payload", "recursive": false, "create_parents": true, "content_hash": "previous-hash"}
			values[tc.field] = tc.value
			if tc.field == "source" {
				delete(values, "content")
			}
			plan := tfsdk.Plan{Schema: s, Raw: resourceValue(s, values)}
			resp := resource.ModifyPlanResponse{Plan: plan}
			r.ModifyPlan(ctx, resource.ModifyPlanRequest{Plan: plan}, &resp)
			if resp.Diagnostics.HasError() {
				t.Fatal(resp.Diagnostics)
			}
			var hash types.String
			if d := resp.Plan.GetAttribute(ctx, path.Root("content_hash"), &hash); d.HasError() {
				t.Fatal(d)
			}
			if hash.IsUnknown() != tc.hashUnknown {
				t.Fatalf("hash=%s want unknown=%t", hash, tc.hashUnknown)
			}
			if !tc.hashUnknown && hash.ValueString() != hashBytes([]byte(values["content"].(string))) {
				t.Fatalf("wrong known hash %s", hash)
			}
		})
	}
}
