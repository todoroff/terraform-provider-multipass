package provider

import (
	"context"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/todoroff/terraform-provider-multipass/internal/multipasscli"
)

var (
	_ datasource.DataSource              = (*instanceDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*instanceDataSource)(nil)
)

// NewInstanceDataSource returns the instance data source.
func NewInstanceDataSource() datasource.DataSource {
	return &instanceDataSource{}
}

type instanceDataSource struct {
	client multipasscli.Client
}

type instanceDataSourceModel struct {
	Name          types.String `tfsdk:"name"`
	State         types.String `tfsdk:"state"`
	Release       types.String `tfsdk:"release"`
	ImageRelease  types.String `tfsdk:"image_release"`
	IPv4          types.List   `tfsdk:"ipv4"`
	CPUCount      types.Int64  `tfsdk:"cpu_count"`
	MemoryTotal   types.Int64  `tfsdk:"memory_total_bytes"`
	MemoryUsed    types.Int64  `tfsdk:"memory_used_bytes"`
	DiskTotal     types.Int64  `tfsdk:"disk_total_bytes"`
	DiskUsed      types.Int64  `tfsdk:"disk_used_bytes"`
	SnapshotCount types.Int64  `tfsdk:"snapshot_count"`
	LastUpdated   types.String `tfsdk:"last_updated"`
}

func (d *instanceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_instance"
}

func (d *instanceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Reads an existing Multipass instance.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Instance name to inspect.",
			},
			"state": schema.StringAttribute{
				Computed: true,
			},
			"release": schema.StringAttribute{
				Computed: true,
			},
			"image_release": schema.StringAttribute{
				Computed: true,
			},
			"ipv4": schema.ListAttribute{
				ElementType: types.StringType,
				Computed:    true,
			},
			"cpu_count": schema.Int64Attribute{
				Computed: true,
			},
			"memory_total_bytes": schema.Int64Attribute{
				Computed: true,
			},
			"memory_used_bytes": schema.Int64Attribute{
				Computed: true,
			},
			"disk_total_bytes": schema.Int64Attribute{
				Computed: true,
			},
			"disk_used_bytes": schema.Int64Attribute{
				Computed: true,
			},
			"snapshot_count": schema.Int64Attribute{
				Computed: true,
			},
			"last_updated": schema.StringAttribute{
				Computed: true,
			},
		},
	}
}

func (d *instanceDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data := req.ProviderData.(providerData)
	d.client = data.client
}

func (d *instanceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError("Client not configured", "Multipass client is nil.")
		return
	}

	var config instanceDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	instance, err := d.client.GetInstance(ctx, config.Name.ValueString())
	if err != nil {
		if err == multipasscli.ErrNotFound {
			resp.Diagnostics.AddError("Instance not found", "The requested Multipass instance does not exist.")
			return
		}
		resp.Diagnostics.AddError("Failed to read instance", err.Error())
		return
	}

	ipv4, diag := types.ListValueFrom(ctx, types.StringType, instance.IPv4)
	resp.Diagnostics.Append(diag...)

	state := instanceDataSourceModel{
		Name:          types.StringValue(instance.Name),
		State:         types.StringValue(instance.State),
		Release:       types.StringValue(instance.Release),
		ImageRelease:  types.StringValue(instance.ImageRelease),
		IPv4:          ipv4,
		CPUCount:      types.Int64Value(int64(instance.CPUCount)),
		MemoryTotal:   types.Int64Value(int64(instance.MemoryTotal)),
		MemoryUsed:    types.Int64Value(int64(instance.MemoryUsed)),
		DiskTotal:     types.Int64Value(int64(instance.DiskTotal)),
		DiskUsed:      types.Int64Value(int64(instance.DiskUsed)),
		SnapshotCount: types.Int64Value(int64(instance.SnapshotCount)),
		LastUpdated:   types.StringValue(instance.LastUpdated.UTC().Format(time.RFC3339)),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}
