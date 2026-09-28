package provider

import (
	"context"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/todoroff/terraform-provider-multipass/internal/models"
	"github.com/todoroff/terraform-provider-multipass/internal/multipasscli"
)

func resourceSchema(r resource.Resource) schema.Schema {
	var resp resource.SchemaResponse
	r.Schema(context.Background(), resource.SchemaRequest{}, &resp)
	return resp.Schema
}

func resourceValue(s schema.Schema, values map[string]any) tftypes.Value {
	typ := s.Type().TerraformType(context.Background()).(tftypes.Object)
	attrs := make(map[string]tftypes.Value, len(typ.AttributeTypes))
	for name, attrType := range typ.AttributeTypes {
		attrs[name] = tftypes.NewValue(attrType, values[name])
	}
	return tftypes.NewValue(typ, attrs)
}

func dynamicValue(t *testing.T, value tftypes.Value) *tfprotov6.DynamicValue {
	t.Helper()
	dynamic, err := tfprotov6.NewDynamicValue(value.Type(), value)
	if err != nil {
		t.Fatal(err)
	}
	return &dynamic
}

// The embedded interface fails on unexpected operations instead of silently
// accepting an unimplemented mock operation.
type testClient struct {
	multipasscli.Client
	launchInstance  func(context.Context, models.LaunchOptions) error
	exec            func(context.Context, string, []string) error
	getInstance     func(context.Context, string) (*models.Instance, error)
	transferCapture func(context.Context, multipasscli.TransferOptions) ([]byte, error)
}

func (c *testClient) LaunchInstance(ctx context.Context, opts models.LaunchOptions) error {
	return c.launchInstance(ctx, opts)
}

func (c *testClient) Exec(ctx context.Context, instance string, command []string) error {
	return c.exec(ctx, instance, command)
}

func (c *testClient) GetInstance(ctx context.Context, name string) (*models.Instance, error) {
	if c.getInstance != nil {
		return c.getInstance(ctx, name)
	}
	return &models.Instance{Name: name, State: "Running", IPv4: []string{"10.0.0.1"}, LastUpdated: time.Now()}, nil
}

func (c *testClient) TransferCapture(ctx context.Context, opts multipasscli.TransferOptions) ([]byte, error) {
	return c.transferCapture(ctx, opts)
}
