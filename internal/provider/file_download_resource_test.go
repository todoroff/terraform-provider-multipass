package provider

import (
	"archive/tar"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
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

func TestFileDownloadDirectoryContents(t *testing.T) {
	for _, source := range []string{"/var/logs", "logs"} {
		t.Run(source, func(t *testing.T) {
			var archive bytes.Buffer
			tw := tar.NewWriter(&archive)
			for _, header := range []*tar.Header{{Name: "./", Typeflag: tar.TypeDir, Mode: 0o755}, {Name: "./app.log", Typeflag: tar.TypeReg, Mode: 0o644, Size: 3}} {
				if err := tw.WriteHeader(header); err != nil {
					t.Fatal(err)
				}
				if header.Size > 0 {
					if _, err := tw.Write([]byte("log")); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := tw.Close(); err != nil {
				t.Fatal(err)
			}
			r := &fileDownloadResource{client: &testClient{
				exec: func(_ context.Context, _ string, args []string) error {
					if args[0] == "tar" && (len(args) != 6 || !reflect.DeepEqual(args[:4], []string{"tar", "-C", source, "-cf"}) || args[5] != ".") {
						t.Errorf("archive must contain directory contents: %v", args)
					}
					return nil
				},
				transferCapture: func(context.Context, multipasscli.TransferOptions) ([]byte, error) { return archive.Bytes(), nil },
			}}
			model := fileDownloadResourceModel{Instance: types.StringValue("vm"), Source: types.StringValue(source), Recursive: types.BoolValue(true), Overwrite: types.BoolValue(true), CreateParents: types.BoolValue(true)}
			dest := filepath.Join(t.TempDir(), "output")
			if d := r.downloadWithTar(context.Background(), &model, dest); d.HasError() {
				t.Fatal(d)
			}
			data, err := os.ReadFile(filepath.Join(dest, "app.log"))
			if err != nil || string(data) != "log" {
				t.Fatalf("wrong directory layout: %q %v", data, err)
			}
		})
	}
}

func TestSanitizeExtractPath(t *testing.T) {
	dest := t.TempDir()
	for _, name := range []string{".", "./app.log", "sub/file.txt", "file..txt"} {
		if _, err := sanitizeExtractPath(dest, name); err != nil {
			t.Errorf("valid path %q: %v", name, err)
		}
	}
	for _, name := range []string{"../outside", "sub/../../outside", string(os.PathSeparator) + "outside"} {
		if _, err := sanitizeExtractPath(dest, name); err == nil {
			t.Errorf("accepted unsafe path %q", name)
		}
	}
}

func TestFileDownloadUpdatePlansNewHash(t *testing.T) {
	ctx := context.Background()
	s := resourceSchema(NewFileDownloadResource())
	values := map[string]any{"id": "download", "instance": "vm", "source": "/tmp/file", "destination": "file", "recursive": false, "create_parents": true, "overwrite": false, "content_hash": "old-hash", "resolved_destination": "/file"}
	prior := resourceValue(s, values)
	values["overwrite"] = true
	next := resourceValue(s, values)
	delete(values, "id")
	delete(values, "content_hash")
	delete(values, "resolved_destination")
	server := providerserver.NewProtocol6(New("test")())()
	resp, err := server.PlanResourceChange(ctx, &tfprotov6.PlanResourceChangeRequest{TypeName: "multipass_file_download", PriorState: dynamicValue(t, prior), ProposedNewState: dynamicValue(t, next), Config: dynamicValue(t, resourceValue(s, values))})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range resp.Diagnostics {
		if d.Severity == tfprotov6.DiagnosticSeverityError {
			t.Fatalf("%s: %s", d.Summary, d.Detail)
		}
	}
	v, err := resp.PlannedState.Unmarshal(next.Type())
	if err != nil {
		t.Fatal(err)
	}
	var attrs map[string]tftypes.Value
	if err := v.As(&attrs); err != nil {
		t.Fatal(err)
	}
	if attrs["content_hash"].IsKnown() {
		t.Fatalf("fresh download hash must be unknown, got %s", attrs["content_hash"])
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
