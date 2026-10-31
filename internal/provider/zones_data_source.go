package provider

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/todoroff/terraform-provider-multipass/internal/models"
	"github.com/todoroff/terraform-provider-multipass/internal/multipasscli"
)

func NewZonesDataSource() datasource.DataSource { return &zonesDataSource{} }

type zonesDataSource struct {
	client   multipasscli.Client
	versions models.VersionInfo
}
type zonesDataSourceModel struct {
	Name  types.String `tfsdk:"name"`
	Zones []zoneModel  `tfsdk:"zones"`
}
type zoneModel struct {
	Name      types.String `tfsdk:"name"`
	Available types.Bool   `tfsdk:"available"`
	Subnet    types.String `tfsdk:"subnet"`
}

func (d *zonesDataSource) Metadata(_ context.Context, _ datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = "multipass_zones"
}
func (d *zonesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{Description: "Lists availability zones. Requires Multipass client and daemon >= 1.17.0.", Attributes: map[string]schema.Attribute{
		"name": schema.StringAttribute{Optional: true, Description: "Optional exact zone name filter."},
		"zones": schema.ListNestedAttribute{Computed: true, NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"name":      schema.StringAttribute{Computed: true},
			"available": schema.BoolAttribute{Computed: true},
			"subnet":    schema.StringAttribute{Computed: true},
		}}},
	}}
}
func (d *zonesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data := req.ProviderData.(providerData)
	d.client = data.client
	d.versions = data.versions
}
func (d *zonesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var state zonesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if d.client == nil {
		resp.Diagnostics.AddError("Client not configured", "Multipass client is nil.")
		return
	}
	if err := multipasscli.RequireZones(d.versions); err != nil {
		resp.Diagnostics.AddError("Availability zones unsupported", err.Error())
		return
	}
	zones, err := d.client.ListZones(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list zones", err.Error())
		return
	}
	state.Zones = []zoneModel{}
	for _, zone := range zones {
		if !state.Name.IsNull() && state.Name.ValueString() != zone.Name {
			continue
		}
		state.Zones = append(state.Zones, zoneModel{types.StringValue(zone.Name), types.BoolValue(zone.Available), types.StringValue(zone.Subnet)})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
