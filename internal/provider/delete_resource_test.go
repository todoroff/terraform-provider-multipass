package provider

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/todoroff/terraform-provider-multipass/internal/models"
	"github.com/todoroff/terraform-provider-multipass/internal/multipasscli"
)

type deleteTestClient struct {
	multipasscli.Client
	err     error
	created bool
}

func (c *deleteTestClient) DeleteInstance(context.Context, string, bool) error         { return c.err }
func (c *deleteTestClient) DeleteSnapshot(context.Context, string, string, bool) error { return c.err }
func (c *deleteTestClient) DeleteAlias(context.Context, string) error                  { return c.err }
func (c *deleteTestClient) CreateAlias(context.Context, models.Alias) error {
	c.created = true
	return nil
}

func TestDeleteMissingResources(t *testing.T) {
	for _, factory := range []struct {
		name   string
		new    func() resource.Resource
		values map[string]any
	}{
		{"instance", NewInstanceResource, map[string]any{"id": "vm", "name": "vm", "networks": []tftypes.Value{}, "mounts": []tftypes.Value{}}},
		{"snapshot", NewSnapshotResource, map[string]any{"id": "vm.snap", "instance": "vm", "name": "snap"}},
		{"alias", NewAliasResource, map[string]any{"id": "default.alias", "context": "default", "name": "alias", "instance": "vm", "command": "ls"}},
	} {
		for _, tc := range []struct {
			name      string
			err       error
			wantError bool
		}{
			{"missing", multipasscli.ErrNotFound, false},
			{"wrapped_missing", fmt.Errorf("CLI: %w", multipasscli.ErrNotFound), false},
			{"other_failure", errors.New("connection failed"), true},
		} {
			t.Run(factory.name+"/"+tc.name, func(t *testing.T) {
				ctx := context.Background()
				r := factory.new()
				r.(resource.ResourceWithConfigure).Configure(ctx, resource.ConfigureRequest{ProviderData: providerData{client: &deleteTestClient{err: tc.err}, commandTimeout: time.Second}}, &resource.ConfigureResponse{})
				s := resourceSchema(r)
				state := tfsdk.State{Schema: s, Raw: resourceValue(s, factory.values)}
				resp := resource.DeleteResponse{State: state}
				r.Delete(ctx, resource.DeleteRequest{State: state}, &resp)
				if resp.Diagnostics.HasError() != tc.wantError {
					t.Fatalf("diagnostics: %v", resp.Diagnostics)
				}
			})
		}
	}
}

func TestAliasUpdateRecreatesMissingAlias(t *testing.T) {
	ctx := context.Background()
	client := &deleteTestClient{err: fmt.Errorf("CLI: %w", multipasscli.ErrNotFound)}
	r := &aliasResource{client: client}
	s := resourceSchema(r)
	plan := tfsdk.Plan{Schema: s, Raw: resourceValue(s, map[string]any{"id": "default.alias", "context": "default", "name": "alias", "instance": "vm", "command": "ls"})}
	resp := resource.UpdateResponse{State: tfsdk.State{Schema: s}}
	r.Update(ctx, resource.UpdateRequest{Plan: plan}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	if !client.created {
		t.Fatal("missing alias was not recreated")
	}
}

func TestUploadDeleteFailureKeepsResource(t *testing.T) {
	for _, tc := range []struct {
		name              string
		execErr, listErr  error
		exists, wantError bool
	}{
		{"permission", errors.New("permission denied"), nil, true, true},
		{"timeout", multipasscli.ErrTimeout, errors.New("offline"), true, true},
		{"missing command", multipasscli.ErrNotFound, nil, true, true},
		{"absent VM", multipasscli.ErrNotFound, nil, false, false},
		{"already absent file", nil, nil, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &fileUploadResource{client: &testClient{exec: func(context.Context, string, []string) error { return tc.execErr }, listInstances: func(context.Context, bool) ([]models.Instance, error) {
				if tc.exists {
					return []models.Instance{{Name: "vm"}}, tc.listErr
				}
				return nil, tc.listErr
			}}}
			s := resourceSchema(r)
			state := tfsdk.State{Schema: s, Raw: resourceValue(s, map[string]any{"id": "vm:/file", "instance": "vm", "destination": "/file", "resolved_destination": "/file", "directory": false})}
			resp := resource.DeleteResponse{State: state}
			r.Delete(context.Background(), resource.DeleteRequest{State: state}, &resp)
			if resp.Diagnostics.HasError() != tc.wantError {
				t.Fatal(resp.Diagnostics)
			}
			if tc.wantError && resp.State.Raw.IsNull() {
				t.Fatal("failed deletion lost state")
			}
		})
	}
}

func (c *deleteTestClient) CurrentAliasContext(context.Context) (string, error) {
	return "default", nil
}
func (c *deleteTestClient) ListAliases(context.Context, bool) ([]models.Alias, error) {
	return []models.Alias{{Name: "alias", Context: "default", Instance: "vm"}}, nil
}
