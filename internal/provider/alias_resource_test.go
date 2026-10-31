package provider

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/todoroff/terraform-provider-multipass/internal/models"
	"github.com/todoroff/terraform-provider-multipass/internal/multipasscli"
	"testing"
)

type contextAliasClient struct {
	multipasscli.Client
	active string
	calls  []string
}

func (c *contextAliasClient) CurrentAliasContext(context.Context) (string, error) {
	return c.active, nil
}
func (c *contextAliasClient) ListAliases(context.Context, bool) ([]models.Alias, error) {
	return []models.Alias{{Context: "dev", Name: "shell", Instance: "dev-vm"}, {Context: "prod", Name: "shell", Instance: "prod-vm"}}, nil
}
func (c *contextAliasClient) DeleteAlias(_ context.Context, name string) error {
	c.calls = append(c.calls, name)
	return nil
}
func (c *contextAliasClient) CreateAlias(_ context.Context, a models.Alias) error {
	c.calls = append(c.calls, a.Context+"."+a.Name)
	return nil
}
func TestAliasContextOwnership(t *testing.T) {
	ctx := context.Background()
	c := &contextAliasClient{active: "prod"}
	r := &aliasResource{client: c}
	s := resourceSchema(r)
	values := map[string]any{"id": "dev.shell", "name": "shell", "context": "dev", "instance": "dev-vm", "command": "bash"}
	state := tfsdk.State{Schema: s, Raw: resourceValue(s, values)}
	read := resource.ReadResponse{State: state}
	r.Read(ctx, resource.ReadRequest{State: state}, &read)
	if read.Diagnostics.HasError() {
		t.Fatal(read.Diagnostics)
	}
	var got aliasResourceModel
	if d := read.State.Get(ctx, &got); d.HasError() {
		t.Fatal(d)
	}
	if got.Instance.ValueString() != "dev-vm" {
		t.Fatal(got)
	}
	update := resource.UpdateResponse{State: state}
	r.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan{Schema: s, Raw: state.Raw}, State: state}, &update)
	if !update.Diagnostics.HasError() || len(c.calls) != 0 {
		t.Fatal("wrong-context update mutated aliases")
	}
	deleted := resource.DeleteResponse{State: state}
	r.Delete(ctx, resource.DeleteRequest{State: state}, &deleted)
	if deleted.Diagnostics.HasError() || len(c.calls) != 1 || c.calls[0] != "dev.shell" {
		t.Fatalf("%v %v", c.calls, deleted.Diagnostics)
	}
	delete(values, "context")
	delete(values, "id")
	delete(values, "instance")
	legacy := tfsdk.State{Schema: s, Raw: resourceValue(s, values)}
	read = resource.ReadResponse{State: legacy}
	r.Read(ctx, resource.ReadRequest{State: legacy}, &read)
	if !read.Diagnostics.HasError() {
		t.Fatal("ambiguous import selected an arbitrary context")
	}
	values["instance"] = "dev-vm"
	legacy.Raw = resourceValue(s, values)
	read = resource.ReadResponse{State: legacy}
	r.Read(ctx, resource.ReadRequest{State: legacy}, &read)
	if read.Diagnostics.HasError() {
		t.Fatal(read.Diagnostics)
	}
	if d := read.State.Get(ctx, &got); d.HasError() {
		t.Fatal(d)
	}
	if got.Context.ValueString() != "dev" || got.ID.ValueString() != "dev.shell" {
		t.Fatal(got)
	}
	c.active = "dev"
	c.calls = nil
	update = resource.UpdateResponse{State: read.State}
	r.Update(ctx, resource.UpdateRequest{Plan: tfsdk.Plan{Schema: s, Raw: read.State.Raw}, State: read.State}, &update)
	if update.Diagnostics.HasError() || len(c.calls) != 2 || c.calls[0] != "dev.shell" || c.calls[1] != "dev.shell" {
		t.Fatalf("%v %v", c.calls, update.Diagnostics)
	}
}
