package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/todoroff/terraform-provider-multipass/internal/models"
	"github.com/todoroff/terraform-provider-multipass/internal/multipasscli"
)

var (
	_ datasource.DataSource              = (*networksDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*networksDataSource)(nil)
)

// NewNetworksDataSource returns the data source definition.
func NewNetworksDataSource() datasource.DataSource {
	return &networksDataSource{}
}

type networksDataSource struct {
	client multipasscli.Client
}

type networksDataSourceModel struct {
	Name     types.String   `tfsdk:"name"`
	Networks []networkModel `tfsdk:"networks"`
}

type networkModel struct {
	Name        types.String `tfsdk:"name"`
	Type        types.String `tfsdk:"type"`
	Description types.String `tfsdk:"description"`
}

func (d *networksDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_networks"
}

func (d *networksDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists host networks available for Multipass bridged attachments.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Optional:    true,
				Description: "Exact network name filter.",
			},
			"networks": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Computed: true,
						},
						"type": schema.StringAttribute{
							Computed: true,
						},
						"description": schema.StringAttribute{
							Computed: true,
						},
					},
				},
			},
		},
	}
}

func (d *networksDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data := req.ProviderData.(providerData)
	d.client = data.client
}

func (d *networksDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError("Client not configured", "Multipass client is nil.")
		return
	}

	var config networksDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	networks, err := d.client.ListNetworks(ctx, false)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list networks", err.Error())
		return
	}

	nameFilter := strings.TrimSpace(config.Name.ValueString())
	var filtered []models.Network
	for _, nw := range networks {
		if nameFilter != "" && nw.Name != nameFilter {
			continue
		}
		filtered = append(filtered, nw)
	}

	model := networksDataSourceModel{
		Name:     config.Name,
		Networks: flattenNetworks(filtered),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func flattenNetworks(networks []models.Network) []networkModel {
	result := make([]networkModel, 0, len(networks))
	for _, nw := range networks {
		result = append(result, networkModel{
			Name:        types.StringValue(nw.Name),
			Type:        types.StringValue(nw.Type),
			Description: types.StringValue(nw.Description),
		})
	}
	return result
}
