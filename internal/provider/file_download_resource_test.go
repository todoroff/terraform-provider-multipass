package provider

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/todoroff/terraform-provider-multipass/internal/multipasscli"
)

func TestFileDownloadOwnsOnlyDownloadedFile(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "new_state", true: "legacy_state"}[legacy], func(t *testing.T) {
			ctx := context.Background()
			dest := filepath.Join(t.TempDir(), "shared")
			if err := os.Mkdir(dest, 0o700); err != nil {
				t.Fatal(err)
			}
			sibling := filepath.Join(dest, "keep.txt")
			if err := os.WriteFile(sibling, []byte("keep"), 0o600); err != nil {
				t.Fatal(err)
			}
			r := &fileDownloadResource{hostOS: "windows", commandTimeout: time.Second, client: &testClient{
				transferCapture: func(context.Context, multipasscli.TransferOptions) ([]byte, error) { return []byte("log data"), nil },
			}}
			s := resourceSchema(r)
			values := map[string]any{"instance": "vm", "source": "/var/log/cloud-init.log", "destination": dest, "recursive": false, "create_parents": true, "overwrite": true}
			create := resource.CreateResponse{State: tfsdk.State{Schema: s}}
			r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: resourceValue(s, values)}}, &create)
			if create.Diagnostics.HasError() {
				t.Fatal(create.Diagnostics)
			}
			owned := filepath.Join(dest, "cloud-init.log")
			if _, err := os.Stat(owned); err != nil {
				t.Fatal(err)
			}
			state := create.State
			if legacy {
				values["id"] = "vm:/var/log/cloud-init.log->" + dest
				state.Raw = resourceValue(s, values)
			}
			deleted := resource.DeleteResponse{State: state}
			r.Delete(ctx, resource.DeleteRequest{State: state}, &deleted)
			if deleted.Diagnostics.HasError() {
				t.Fatal(deleted.Diagnostics)
			}
			if _, err := os.Stat(owned); !os.IsNotExist(err) {
				t.Fatalf("download still exists: %v", err)
			}
			if data, err := os.ReadFile(sibling); err != nil || string(data) != "keep" {
				t.Fatalf("unrelated file lost: %q %v", data, err)
			}
		})
	}
}

func TestFileDownloadReadDetectsMissingChild(t *testing.T) {
	ctx := context.Background()
	dest := t.TempDir()
	r := &fileDownloadResource{client: &testClient{}}
	s := resourceSchema(r)
	state := tfsdk.State{Schema: s, Raw: resourceValue(s, map[string]any{"id": "download", "instance": "vm", "source": "/tmp/log.txt", "destination": dest, "recursive": false})}
	resp := resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if !resp.State.Raw.IsNull() {
		t.Fatal("missing child must remove resource even while destination directory exists")
	}
}
