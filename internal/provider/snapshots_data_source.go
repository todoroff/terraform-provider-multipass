package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/todoroff/terraform-provider-multipass/internal/multipasscli"
)

var (
	_ datasource.DataSource              = (*snapshotsDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*snapshotsDataSource)(nil)
)

// NewSnapshotsDataSource returns the snapshots data source.
func NewSnapshotsDataSource() datasource.DataSource {
	return &snapshotsDataSource{}
}

type snapshotsDataSource struct {
	client multipasscli.Client
}

type snapshotsDataSourceModel struct {
	Instance  types.String        `tfsdk:"instance"`
	Name      types.String        `tfsdk:"name"`
	Snapshots []snapshotModelInfo `tfsdk:"snapshots"`
}

type snapshotModelInfo struct {
	Instance types.String `tfsdk:"instance"`
	Name     types.String `tfsdk:"name"`
	Comment  types.String `tfsdk:"comment"`
	Parent   types.String `tfsdk:"parent"`
}

func (d *snapshotsDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_snapshots"
}

func (d *snapshotsDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists snapshots for a given Multipass instance.",
		Attributes: map[string]schema.Attribute{
			"instance": schema.StringAttribute{
				Required:    true,
				Description: "Instance name to list snapshots for.",
			},
			"name": schema.StringAttribute{
				Optional:    true,
				Description: "Optional snapshot name filter.",
			},
			"snapshots": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"instance": schema.StringAttribute{
							Computed: true,
						},
						"name": schema.StringAttribute{
							Computed: true,
						},
						"comment": schema.StringAttribute{
							Computed: true,
						},
						"parent": schema.StringAttribute{
							Computed: true,
						},
					},
				},
			},
		},
	}
}

func (d *snapshotsDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data := req.ProviderData.(providerData)
	d.client = data.client
}

func (d *snapshotsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError("Client not configured", "Multipass client is nil.")
		return
	}

	var config snapshotsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	instance := config.Instance.ValueString()
	nameFilter := ""
	if !config.Name.IsNull() && !config.Name.IsUnknown() {
		nameFilter = config.Name.ValueString()
	}

	snapshots, err := d.client.ListSnapshots(ctx, instance)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list snapshots", err.Error())
		return
	}

	result := make([]snapshotModelInfo, 0, len(snapshots))
	for _, s := range snapshots {
		if nameFilter != "" && s.Name != nameFilter {
			continue
		}
		result = append(result, snapshotModelInfo{
			Instance: types.StringValue(s.Instance),
			Name:     types.StringValue(s.Name),
			Comment:  types.StringValue(s.Comment),
			Parent:   types.StringValue(s.Parent),
		})
	}

	state := snapshotsDataSourceModel{
		Instance:  config.Instance,
		Name:      config.Name,
		Snapshots: result,
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}


