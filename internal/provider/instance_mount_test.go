package provider

import (
	"context"
	"errors"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/todoroff/terraform-provider-multipass/internal/models"
	"github.com/todoroff/terraform-provider-multipass/internal/multipasscli"
	"reflect"
	"testing"
	"time"
)

type auditMountClient struct {
	multipasscli.Client
	calls []string
}

func (c *auditMountClient) Unmount(context.Context, string, models.Mount) error {
	c.calls = append(c.calls, "unmount all")
	return nil
}
func (c *auditMountClient) Mount(context.Context, string, models.Mount) error {
	c.calls = append(c.calls, "mount")
	if len(c.calls) == 2 {
		return errors.New("new host path unavailable")
	}
	return nil
}
func TestFailedMountUpdateCanBeReverted(t *testing.T) {
	ctx := context.Background()
	f := newResizeFixture()
	c := &auditMountClient{Client: f.client}
	r := &instanceResource{client: c, commandTimeout: time.Second}
	s := resourceSchema(r)
	typ := s.Type().TerraformType(ctx).(tftypes.Object).AttributeTypes["mounts"].(tftypes.List).ElementType
	mount := func(host string) []tftypes.Value {
		return []tftypes.Value{tftypes.NewValue(typ, map[string]tftypes.Value{"host_path": tftypes.NewValue(tftypes.String, host), "instance_path": tftypes.NewValue(tftypes.String, "/data"), "read_only": tftypes.NewValue(tftypes.Bool, false)})}
	}
	values := resizeValues()
	values["mounts"] = mount("/old-host-data")
	state := tfsdk.State{Schema: s, Raw: resourceValue(s, values)}
	values["mounts"] = mount("/missing-new-host-data")
	resp := resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{State: state, Plan: tfsdk.Plan{Schema: s, Raw: resourceValue(s, values)}}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("expected failed mount")
	}
	read := resource.ReadResponse{State: resp.State}
	r.Read(ctx, resource.ReadRequest{State: resp.State}, &read)
	if read.Diagnostics.HasError() {
		t.Fatal(read.Diagnostics)
	}
	values["mounts"] = mount("/old-host-data")
	retry := resource.UpdateResponse{State: read.State}
	r.Update(ctx, resource.UpdateRequest{State: read.State, Plan: tfsdk.Plan{Schema: s, Raw: resourceValue(s, values)}}, &retry)
	if retry.Diagnostics.HasError() {
		t.Fatal(retry.Diagnostics)
	}
	if !reflect.DeepEqual(c.calls, []string{"unmount all", "mount", "unmount all", "mount"}) {
		t.Fatal(c.calls)
	}
	t.Logf("reverting to original config restores the mount; all calls=%v", c.calls)
}
