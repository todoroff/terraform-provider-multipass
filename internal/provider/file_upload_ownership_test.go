package provider

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/todoroff/terraform-provider-multipass/internal/multipasscli"
)

// This fixture applies Multipass's basename rule to transfers into directories.
type uploadOwnershipClient struct {
	multipasscli.Client
	paths map[string]string
	calls [][]string
}

func (c *uploadOwnershipClient) ExecCapture(_ context.Context, _ string, args []string) ([]byte, error) {
	if args[0] == "mktemp" {
		c.paths["/tmp/multipass-upload.fixture"] = "directory"
		return []byte("/tmp/multipass-upload.fixture\n"), nil
	}
	kind := "missing"
	if value, ok := c.paths[args[len(args)-1]]; ok {
		kind = "file"
		if value == "directory" {
			kind = value
		}
	}
	return []byte(kind), nil
}
func (c *uploadOwnershipClient) Transfer(_ context.Context, opts multipasscli.TransferOptions) error {
	_, dest, _ := strings.Cut(opts.Destination, ":")
	base := "stream_output.dat"
	data := string(opts.Stdin)
	if len(opts.Sources) > 0 {
		base = filepath.Base(opts.Sources[0])
		if !opts.Recursive {
			b, err := os.ReadFile(opts.Sources[0])
			if err != nil {
				return err
			}
			data = string(b)
		}
	}
	if c.paths[dest] == "directory" {
		dest = path.Join(dest, base)
	}
	if opts.Recursive {
		c.paths[dest] = "directory"
		c.paths[path.Join(dest, "payload")] = "uploaded"
	} else {
		c.paths[dest] = data
	}
	return nil
}
func (c *uploadOwnershipClient) Exec(_ context.Context, _ string, args []string) error {
	c.calls = append(c.calls, args)
	switch args[0] {
	case "rm":
		dest := args[len(args)-1]
		if args[1] == "-f" && c.paths[dest] == "directory" {
			return fmt.Errorf("refusing directory")
		}
		for p := range c.paths {
			if p == dest || (args[1] == "-rf" && strings.HasPrefix(p, dest+"/")) {
				delete(c.paths, p)
			}
		}
	case "cp":
		src := strings.TrimSuffix(args[3], "/.")
		dest := args[4]
		for p, v := range c.paths {
			if strings.HasPrefix(p, src+"/") {
				c.paths[dest+strings.TrimPrefix(p, src)] = v
			}
		}
	default:
		return fmt.Errorf("unexpected command %v", args)
	}
	return nil
}
func TestUploadOwnsOnlyTransferredPath(t *testing.T) {
	for _, kind := range []string{"file", "inline", "directory"} {
		t.Run(kind, func(t *testing.T) {
			c := &uploadOwnershipClient{paths: map[string]string{"/shared": "directory", "/shared/keep": "unrelated"}}
			r := &fileUploadResource{client: c, commandTimeout: time.Second}
			s := resourceSchema(r)
			source := filepath.Join(t.TempDir(), "payload")
			if kind == "directory" {
				if err := os.Mkdir(source, 0700); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := os.WriteFile(source, []byte("file"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			values := map[string]any{"instance": "vm", "destination": "/shared", "recursive": kind == "directory", "create_parents": true}
			expected := "/shared/payload"
			if kind == "inline" {
				values["content"] = "inline"
				expected = "/shared/stream_output.dat"
			} else {
				values["source"] = source
			}
			created := resource.CreateResponse{State: tfsdk.State{Schema: s}}
			r.Create(context.Background(), resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: resourceValue(s, values)}}, &created)
			if created.Diagnostics.HasError() {
				t.Fatal(created.Diagnostics)
			}
			var model fileUploadResourceModel
			if d := created.State.Get(context.Background(), &model); d.HasError() {
				t.Fatal(d)
			}
			if model.ResolvedDestination.ValueString() != expected {
				t.Fatal(model.ResolvedDestination)
			}
			if kind == "directory" { // Repeated transfers must remain within the recorded root.
				if d := r.transferUpload(context.Background(), &model); d.HasError() {
					t.Fatal(d)
				}
				if c.paths[expected+"/payload"] != "uploaded" {
					t.Fatal(c.paths)
				}
				if _, nested := c.paths[expected+"/payload/payload"]; nested {
					t.Fatal("update nested the directory")
				}
			}
			deleted := resource.DeleteResponse{State: created.State}
			r.Delete(context.Background(), resource.DeleteRequest{State: created.State}, &deleted)
			if deleted.Diagnostics.HasError() {
				t.Fatal(deleted.Diagnostics)
			}
			flags := "-f"
			if kind == "directory" {
				flags = "-rf"
			}
			if !reflect.DeepEqual(c.calls[len(c.calls)-1], []string{"rm", flags, "--", expected}) {
				t.Fatal(c.calls)
			}
			if !reflect.DeepEqual(c.paths, map[string]string{"/shared": "directory", "/shared/keep": "unrelated"}) {
				t.Fatal(c.paths)
			}
		})
	}
}
func TestUploadLegacyOwnershipAndImport(t *testing.T) {
	c := &uploadOwnershipClient{paths: map[string]string{"/shared": "directory", "/shared/file": "data"}}
	r := &fileUploadResource{client: c}
	ctx := context.Background()
	m := fileUploadResourceModel{Instance: types.StringValue("vm"), Destination: types.StringValue("/shared"), Source: types.StringValue("file"), Recursive: types.BoolValue(false)}
	if err := r.resolveLegacyUpload(ctx, &m); err != nil || m.ResolvedDestination.ValueString() != "/shared/file" || m.Directory.ValueBool() {
		t.Fatalf("%+v %v", m, err)
	}
	m.ResolvedDestination = types.StringNull()
	m.Recursive = types.BoolValue(true)
	if err := r.resolveLegacyUpload(ctx, &m); err == nil {
		t.Fatal("ambiguous legacy directory accepted")
	}
	s := resourceSchema(r)
	imp := resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: resourceValue(s, map[string]any{})}}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "vm:/shared/owned"}, &imp)
	if imp.Diagnostics.HasError() {
		t.Fatal(imp.Diagnostics)
	}
	if d := imp.State.Get(ctx, &m); d.HasError() {
		t.Fatal(d)
	}
	c.paths["/shared/owned"] = "directory"
	if err := r.resolveLegacyUpload(ctx, &m); err != nil || !m.Directory.ValueBool() {
		t.Fatalf("%+v %v", m, err)
	}
}

func TestUploadDirectoryRetainsRenamedRoot(t *testing.T) {
	c := &uploadOwnershipClient{paths: map[string]string{"/shared": "directory", "/shared/keep": "unrelated"}}
	r := &fileUploadResource{client: c}
	src := filepath.Join(t.TempDir(), "source-name")
	if err := os.Mkdir(src, 0700); err != nil {
		t.Fatal(err)
	}
	m := fileUploadResourceModel{Instance: types.StringValue("vm"), Source: types.StringValue(src), Destination: types.StringValue("/shared/renamed"), Recursive: types.BoolValue(true), CreateParents: types.BoolValue(true)}
	for i := 0; i < 2; i++ {
		if d := r.transferUpload(context.Background(), &m); d.HasError() {
			t.Fatal(d)
		}
	}
	want := map[string]string{"/shared": "directory", "/shared/keep": "unrelated", "/shared/renamed": "directory", "/shared/renamed/payload": "uploaded"}
	if !reflect.DeepEqual(c.paths, want) {
		t.Fatal(c.paths)
	}
}

type interruptedUploadClient struct {
	*uploadOwnershipClient
	interrupt         func(context.Context)
	cleanupErr        error
	cleanupContextErr error
	cleanupDeadline   time.Time
}

func (c *interruptedUploadClient) Transfer(ctx context.Context, opts multipasscli.TransferOptions) error {
	if err := c.uploadOwnershipClient.Transfer(ctx, opts); err != nil {
		return err
	}
	c.interrupt(ctx)
	return ctx.Err()
}

func (c *interruptedUploadClient) Exec(ctx context.Context, instance string, args []string) error {
	c.cleanupContextErr = ctx.Err()
	c.cleanupDeadline, _ = ctx.Deadline()
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.cleanupErr != nil {
		return c.cleanupErr
	}
	return c.uploadOwnershipClient.Exec(ctx, instance, args)
}

func TestUploadCleansStagingAfterInterruption(t *testing.T) {
	for _, kind := range []string{"cancel", "timeout", "cleanup failure"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			interrupt := func(context.Context) { cancel() }
			if kind == "timeout" {
				cancel()
				ctx, cancel = context.WithTimeout(context.Background(), 10*time.Millisecond)
				interrupt = func(ctx context.Context) { <-ctx.Done() }
			}
			defer cancel()
			c := &interruptedUploadClient{
				uploadOwnershipClient: &uploadOwnershipClient{paths: map[string]string{"/owned": "directory", "/owned/keep": "previous"}},
				interrupt:             interrupt,
			}
			if kind == "cleanup failure" {
				c.cleanupErr = errors.New("guest unavailable")
			}
			r := &fileUploadResource{client: c}
			m := fileUploadResourceModel{Instance: types.StringValue("vm"), Source: types.StringValue(t.TempDir()), Destination: types.StringValue("/owned"), ResolvedDestination: types.StringValue("/owned"), Directory: types.BoolValue(true), Recursive: types.BoolValue(true)}
			before := time.Now()
			d := r.transferUpload(ctx, &m)
			if !d.HasError() || d[0].Detail() != ctx.Err().Error() {
				t.Fatalf("original transfer error lost: %v", d)
			}
			if c.cleanupContextErr != nil || !c.cleanupDeadline.After(before) || c.cleanupDeadline.After(time.Now().Add(30*time.Second)) {
				t.Fatalf("cleanup context: err=%v deadline=%v", c.cleanupContextErr, c.cleanupDeadline)
			}
			if c.cleanupErr == nil {
				want := map[string]string{"/owned": "directory", "/owned/keep": "previous"}
				if !reflect.DeepEqual(c.paths, want) || d.WarningsCount() != 0 {
					t.Fatalf("staging not cleaned: %v %v", c.paths, d)
				}
			} else if d.WarningsCount() != 1 || !strings.Contains(d[1].Detail(), c.cleanupErr.Error()) {
				t.Fatalf("cleanup failure not reported: %v", d)
			}
		})
	}
}
