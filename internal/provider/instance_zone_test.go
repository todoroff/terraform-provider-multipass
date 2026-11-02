package provider

import (
	"context"
	"errors"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/todoroff/terraform-provider-multipass/internal/models"
	"github.com/todoroff/terraform-provider-multipass/internal/multipasscli"
	"testing"
	"time"
)

var zoneVersions = models.VersionInfo{Client: "1.17.0", Daemon: "1.17.0"}

func TestZoneReplacementPlan(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		prior, config, proposed any
		replace                 bool
	}{
		{"change", "zone1", "zone2", "zone2", true},
		{"same", "zone1", "zone1", "zone1", false},
		{"remove", "zone1", nil, "zone1", false},
		{"unconfigured", "zone2", nil, "zone2", false},
		{"legacy", nil, nil, nil, false},
		{"legacy explicit", nil, "zone2", "zone2", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			s := resourceSchema(NewInstanceResource())
			values := resizeValues()
			values["zone"] = tc.prior
			prior := resourceValue(s, values)
			values["zone"] = tc.proposed
			next := resourceValue(s, values)
			values["zone"] = tc.config
			delete(values, "id")
			delete(values, "state")
			server := providerserver.NewProtocol6(New("test")())()
			resp, err := server.PlanResourceChange(ctx, &tfprotov6.PlanResourceChangeRequest{TypeName: "multipass_instance", PriorState: dynamicValue(t, prior), ProposedNewState: dynamicValue(t, next), Config: dynamicValue(t, resourceValue(s, values))})
			if err != nil {
				t.Fatal(err)
			}
			for _, d := range resp.Diagnostics {
				if d.Severity == tfprotov6.DiagnosticSeverityError {
					t.Fatalf("%+v", d)
				}
			}
			if (len(resp.RequiresReplace) > 0) != tc.replace {
				t.Fatalf("replacement: %v", resp.RequiresReplace)
			}
		})
	}
}

func TestZoneCreateAndVersionGate(t *testing.T) {
	for _, tc := range []struct {
		name     string
		versions models.VersionInfo
		zone     any
		wantZone string
		fail     bool
	}{
		{"explicit", zoneVersions, "zone2", "zone2", false},
		{"empty", zoneVersions, "", "", true},
		{"whitespace", zoneVersions, "zone 2", "", true},
		{"automatic", zoneVersions, tftypes.UnknownValue, "", false},
		{"old without zones", models.VersionInfo{Client: "1.16.1", Daemon: "1.16.1"}, nil, "", false},
		{"old daemon", models.VersionInfo{Client: "1.17.0", Daemon: "1.16.1"}, "zone2", "", true},
		{"unknown daemon", models.VersionInfo{Client: "1.17.0"}, "zone2", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			calls := 0
			c := &testClient{launchInstance: func(_ context.Context, opts models.LaunchOptions) error {
				calls++
				if opts.Zone != tc.wantZone {
					t.Fatalf("zone %q", opts.Zone)
				}
				return nil
			}, getInstance: func(context.Context, string) (*models.Instance, error) {
				return &models.Instance{Name: "vm", Zone: "zone2", State: "Running", LastUpdated: time.Now()}, nil
			}}
			r := &instanceResource{client: c, versions: tc.versions, commandTimeout: time.Second}
			s := resourceSchema(r)
			values := map[string]any{"name": "vm", "zone": tc.zone, "networks": []tftypes.Value{}, "mounts": []tftypes.Value{}}
			resp := resource.CreateResponse{State: tfsdk.State{Schema: s}}
			r.Create(ctx, resource.CreateRequest{Plan: tfsdk.Plan{Schema: s, Raw: resourceValue(s, values)}}, &resp)
			if resp.Diagnostics.HasError() != tc.fail || (calls == 0) != tc.fail {
				t.Fatalf("calls=%d diagnostics=%v", calls, resp.Diagnostics)
			}
			if !tc.fail {
				var state instanceResourceModel
				if d := resp.State.Get(ctx, &state); d.HasError() || state.Zone.ValueString() != "zone2" {
					t.Fatalf("state %+v %v", state, d)
				}
			}
		})
	}
}

func TestUnavailableZonePreservesInstance(t *testing.T) {
	for _, readErr := range []error{nil, errors.New("SSH unavailable"), multipasscli.ErrNotFound} {
		ctx := context.Background()
		unavailable := models.Instance{Name: "vm", Zone: "zone2", State: "Unavailable"}
		c := &testClient{getInstance: func(context.Context, string) (*models.Instance, error) { return &unavailable, readErr }, listInstances: func(context.Context, bool) ([]models.Instance, error) { return []models.Instance{unavailable}, nil }, getResources: func(context.Context, string) (models.InstanceResources, error) {
			t.Fatal("must not read unavailable allocations")
			return models.InstanceResources{}, nil
		}}
		r := &instanceResource{client: c, versions: zoneVersions, commandTimeout: time.Second}
		s := resourceSchema(r)
		values := resizeValues()
		values["zone"] = "zone2"
		values["release"] = "Ubuntu"
		values["auto_recover"] = true
		state := tfsdk.State{Schema: s, Raw: resourceValue(s, values)}
		resp := resource.ReadResponse{State: state}
		r.Read(ctx, resource.ReadRequest{State: state}, &resp)
		var got instanceResourceModel
		if resp.Diagnostics.HasError() {
			t.Fatal(resp.Diagnostics)
		}
		if d := resp.State.Get(ctx, &got); d.HasError() {
			t.Fatal(d)
		}
		if got.ID.ValueString() != "vm" || got.State.ValueString() != "Unavailable" || got.Zone.ValueString() != "zone2" || got.Release.ValueString() != "Ubuntu" || got.Memory.ValueString() != values["memory"] {
			t.Fatalf("lost state: %+v", got)
		}
		update := resource.UpdateResponse{State: resp.State}
		r.Update(ctx, resource.UpdateRequest{State: resp.State, Plan: tfsdk.Plan{Schema: s, Raw: resourceValue(s, values)}}, &update)
		if !update.Diagnostics.HasError() || update.State.Raw.IsNull() {
			t.Fatalf("update: %+v", update)
		}
	}
}

type zoneTestClient struct {
	*testClient
	listZones func(context.Context) ([]models.Zone, error)
}

func (c *zoneTestClient) ListZones(ctx context.Context) ([]models.Zone, error) {
	return c.listZones(ctx)
}

func TestZonesDataSource(t *testing.T) {
	for _, filter := range []any{nil, "zone2", "absent"} {
		ctx := context.Background()
		d := &zonesDataSource{versions: zoneVersions, client: &zoneTestClient{listZones: func(context.Context) ([]models.Zone, error) {
			return []models.Zone{{Name: "zone1", Available: true, Subnet: "10.0.1.0/24"}, {Name: "zone2", Subnet: "10.0.2.0/24"}}, nil
		}}}
		var schemaResp datasource.SchemaResponse
		d.Schema(ctx, datasource.SchemaRequest{}, &schemaResp)
		s := schemaResp.Schema
		typ := s.Type().TerraformType(ctx).(tftypes.Object)
		config := tfsdk.Config{Schema: s, Raw: tftypes.NewValue(typ, map[string]tftypes.Value{"name": tftypes.NewValue(tftypes.String, filter), "zones": tftypes.NewValue(typ.AttributeTypes["zones"], nil)})}
		resp := datasource.ReadResponse{State: tfsdk.State{Schema: s}}
		d.Read(ctx, datasource.ReadRequest{Config: config}, &resp)
		if resp.Diagnostics.HasError() {
			t.Fatal(resp.Diagnostics)
		}
		var state zonesDataSourceModel
		if ds := resp.State.Get(ctx, &state); ds.HasError() {
			t.Fatal(ds)
		}
		want := 2
		if filter == "zone2" {
			want = 1
		}
		if filter == "absent" {
			want = 0
		}
		if len(state.Zones) != want {
			t.Fatal(state)
		}
		d.versions = models.VersionInfo{}
		d.client = &zoneTestClient{}
		resp = datasource.ReadResponse{State: tfsdk.State{Schema: s}}
		d.Read(ctx, datasource.ReadRequest{Config: config}, &resp)
		if !resp.Diagnostics.HasError() {
			t.Fatal("unsupported zone listing accepted")
		}
	}
}

func TestZoneImportAndUnavailableDataSource(t *testing.T) {
	ctx := context.Background()
	c := &testClient{getInstance: func(context.Context, string) (*models.Instance, error) {
		return &models.Instance{Name: "vm", Zone: "zone2", State: "Running", LastUpdated: time.Now()}, nil
	}}
	r := &instanceResource{client: c, versions: zoneVersions, commandTimeout: time.Second}
	s := resourceSchema(r)
	imported := resource.ImportStateResponse{State: tfsdk.State{Schema: s, Raw: resourceValue(s, map[string]any{})}}
	r.ImportState(ctx, resource.ImportStateRequest{ID: "vm"}, &imported)
	read := resource.ReadResponse{State: imported.State}
	r.Read(ctx, resource.ReadRequest{State: imported.State}, &read)
	var got instanceResourceModel
	if read.Diagnostics.HasError() {
		t.Fatal(read.Diagnostics)
	}
	if ds := read.State.Get(ctx, &got); ds.HasError() {
		t.Fatal(ds)
	}
	if got.Zone.ValueString() != "zone2" || got.ID.ValueString() != "vm" {
		t.Fatal(got)
	}
	d := &instanceDataSource{versions: zoneVersions, client: &testClient{
		getInstance: func(context.Context, string) (*models.Instance, error) { return nil, errors.New("zone unavailable") },
		listInstances: func(context.Context, bool) ([]models.Instance, error) {
			return []models.Instance{{Name: "vm", Zone: "zone2", State: "Unavailable", LastUpdated: time.Now()}}, nil
		},
	}}
	var sr datasource.SchemaResponse
	d.Schema(ctx, datasource.SchemaRequest{}, &sr)
	typ := sr.Schema.Type().TerraformType(ctx).(tftypes.Object)
	attrs := map[string]tftypes.Value{}
	for name, attrType := range typ.AttributeTypes {
		attrs[name] = tftypes.NewValue(attrType, nil)
	}
	attrs["name"] = tftypes.NewValue(tftypes.String, "vm")
	resp := datasource.ReadResponse{State: tfsdk.State{Schema: sr.Schema}}
	d.Read(ctx, datasource.ReadRequest{Config: tfsdk.Config{Schema: sr.Schema, Raw: tftypes.NewValue(typ, attrs)}}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	var result instanceDataSourceModel
	if ds := resp.State.Get(ctx, &result); ds.HasError() {
		t.Fatal(ds)
	}
	if result.Zone.ValueString() != "zone2" || result.State.ValueString() != "Unavailable" || !result.MemoryTotal.IsNull() || !result.CPUCount.IsNull() {
		t.Fatal(result)
	}
}

func TestZonePlanVersionGate(t *testing.T) {
	ctx := context.Background()
	r := &instanceResource{client: &testClient{}, versions: models.VersionInfo{Client: "1.17.0", Daemon: "1.16.1"}}
	s := resourceSchema(r)
	values := resizeValues()
	values["zone"] = "zone2"
	resp := resource.ModifyPlanResponse{}
	r.ModifyPlan(ctx, resource.ModifyPlanRequest{Config: tfsdk.Config{Schema: s, Raw: resourceValue(s, values)}, Plan: tfsdk.Plan{Schema: s, Raw: resourceValue(s, values)}, State: tfsdk.State{Schema: s, Raw: resourceValue(s, resizeValues())}}, &resp)
	if !resp.Diagnostics.HasError() {
		t.Fatal("unsupported replacement allowed")
	}
}
