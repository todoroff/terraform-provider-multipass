package provider

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestLegacyRecursiveUploadUpgrade(t *testing.T) {
	for _, changed := range []bool{false, true} {
		t.Run(map[bool]string{false: "unchanged", true: "edited"}[changed], func(t *testing.T) {
			ctx := context.Background()
			src := filepath.Join(t.TempDir(), "bundle")
			if err := os.Mkdir(src, 0o700); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(src, "payload")
			if err := os.WriteFile(file, []byte("original"), 0o600); err != nil {
				t.Fatal(err)
			}
			// The 2.1.0 hash was SHA256(relative name + hex content hash).
			oldHash := hashBytes([]byte("payload" + hashBytes([]byte("original"))))
			if changed {
				if err := os.WriteFile(file, []byte("edited"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			c := &uploadOwnershipClient{paths: map[string]string{"/shared": "directory", "/shared/keep": "unrelated", "/shared/bundle": "directory", "/shared/bundle/payload": "original"}}
			r := &fileUploadResource{client: c}
			s := resourceSchema(r)
			values := map[string]any{"id": "vm:/shared", "instance": "vm", "destination": "/shared", "source": src, "recursive": true, "create_parents": true, "content_hash": oldHash}
			state := tfsdk.State{Schema: s, Raw: resourceValue(s, values)}
			read := resource.ReadResponse{State: state}
			r.Read(ctx, resource.ReadRequest{State: state}, &read)
			if read.Diagnostics.HasError() || read.Diagnostics.WarningsCount() != 1 {
				t.Fatal(read.Diagnostics)
			}
			var model fileUploadResourceModel
			if d := read.State.Get(ctx, &model); d.HasError() {
				t.Fatal(d)
			}
			newHash, err := hashDirectory(src)
			if err != nil {
				t.Fatal(err)
			}
			wantHash := newHash
			if changed {
				wantHash = oldHash
			}
			if model.ContentHash.ValueString() != wantHash || !model.ResolvedDestination.IsNull() {
				t.Fatalf("incorrect migrated state: %+v", model)
			}
			delete(values, "id")
			delete(values, "content_hash")
			server := providerserver.NewProtocol6(New("test")())()
			plan, err := server.PlanResourceChange(ctx, &tfprotov6.PlanResourceChangeRequest{TypeName: "multipass_file_upload", PriorState: dynamicValue(t, read.State.Raw), ProposedNewState: dynamicValue(t, read.State.Raw), Config: dynamicValue(t, resourceValue(s, values))})
			if err != nil {
				t.Fatal(err)
			}
			for _, d := range plan.Diagnostics {
				if d.Severity == tfprotov6.DiagnosticSeverityError {
					t.Fatal(d)
				}
			}
			planned, err := plan.PlannedState.Unmarshal(read.State.Raw.Type())
			if err != nil {
				t.Fatal(err)
			}
			if planned.Equal(read.State.Raw) == changed || len(plan.RequiresReplace) != 0 {
				t.Fatalf("unexpected migration plan: %s", planned)
			}
			if changed {
				var attrs map[string]tftypes.Value
				if err := planned.As(&attrs); err != nil {
					t.Fatal(err)
				}
				if attrs["resolved_destination"].IsKnown() {
					t.Fatal("updated upload ownership must be unknown until transfer")
				}
			}
			deleted := resource.DeleteResponse{State: read.State}
			r.Delete(ctx, resource.DeleteRequest{State: read.State}, &deleted)
			if deleted.Diagnostics.HasError() || deleted.Diagnostics.WarningsCount() != 1 || len(c.calls) != 0 || c.paths["/shared/bundle/payload"] != "original" {
				t.Fatalf("ambiguous files not retained: %v %v", deleted.Diagnostics, c.paths)
			}
		})
	}
}

func TestLegacyRecursiveUpdateRecordsOnlyNewTransfer(t *testing.T) {
	ctx := context.Background()
	src := filepath.Join(t.TempDir(), "bundle")
	if err := os.Mkdir(src, 0o700); err != nil {
		t.Fatal(err)
	}
	// An older first upload created /root directly. The next transfer, like
	// Multipass 2.1.0 updates, targets /root/bundle and cannot claim /root.
	c := &uploadOwnershipClient{paths: map[string]string{"/root": "directory", "/root/old": "retained"}}
	r := &fileUploadResource{client: c}
	m := fileUploadResourceModel{ID: types.StringValue("vm:/root"), Instance: types.StringValue("vm"), Destination: types.StringValue("/root"), Source: types.StringValue(src), Recursive: types.BoolValue(true)}
	if d := r.transferUpload(ctx, &m); d.HasError() {
		t.Fatal(d)
	}
	if m.ResolvedDestination.ValueString() != "/root/bundle" || !m.Directory.ValueBool() {
		t.Fatalf("wrong ownership: %+v", m)
	}
	s := resourceSchema(r)
	state := tfsdk.State{Schema: s, Raw: resourceValue(s, map[string]any{"id": "vm:/root", "instance": "vm", "destination": "/root", "resolved_destination": m.ResolvedDestination.ValueString(), "directory": true, "recursive": true})}
	deleted := resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, &deleted)
	if deleted.Diagnostics.HasError() || !reflect.DeepEqual(c.paths, map[string]string{"/root": "directory", "/root/old": "retained"}) {
		t.Fatalf("legacy root was removed: %v %v", deleted.Diagnostics, c.paths)
	}
}

func TestLegacyRecursivePlansChangedInputs(t *testing.T) {
	for _, field := range []string{"source", "create_parents"} {
		t.Run(field, func(t *testing.T) {
			ctx := context.Background()
			s := resourceSchema(NewFileUploadResource())
			values := map[string]any{"id": "vm:/shared", "instance": "vm", "destination": "/shared", "source": t.TempDir(), "recursive": true, "create_parents": true, "content_hash": hashBytes(nil)}
			prior := resourceValue(s, values)
			if field == "source" {
				values[field] = tftypes.UnknownValue
			} else {
				values[field] = false
			}
			next := resourceValue(s, values)
			delete(values, "id")
			delete(values, "content_hash")
			server := providerserver.NewProtocol6(New("test")())()
			resp, err := server.PlanResourceChange(ctx, &tfprotov6.PlanResourceChangeRequest{TypeName: "multipass_file_upload", PriorState: dynamicValue(t, prior), ProposedNewState: dynamicValue(t, next), Config: dynamicValue(t, resourceValue(s, values))})
			if err != nil {
				t.Fatal(err)
			}
			for _, d := range resp.Diagnostics {
				if d.Severity == tfprotov6.DiagnosticSeverityError {
					t.Fatal(d)
				}
			}
			planned, err := resp.PlannedState.Unmarshal(next.Type())
			if err != nil {
				t.Fatal(err)
			}
			var attrs map[string]tftypes.Value
			if err := planned.As(&attrs); err != nil {
				t.Fatal(err)
			}
			if attrs["resolved_destination"].IsKnown() || attrs["directory"].IsKnown() {
				t.Fatal("legacy ownership must be recomputed on update", planned)
			}
		})
	}
}
