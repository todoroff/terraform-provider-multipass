package provider

import (
	"context"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/todoroff/terraform-provider-multipass/internal/models"
	"github.com/todoroff/terraform-provider-multipass/internal/multipasscli"
)

var _ datasource.DataSource = (*imagesDataSource)(nil)
var _ datasource.DataSourceWithConfigure = (*imagesDataSource)(nil)

// NewImagesDataSource creates the data source.
func NewImagesDataSource() datasource.DataSource {
	return &imagesDataSource{}
}

type imagesDataSource struct {
	client multipasscli.Client
}

type imagesDataSourceModel struct {
	Name   types.String `tfsdk:"name"`
	Alias  types.String `tfsdk:"alias"`
	Kind   types.String `tfsdk:"kind"`
	Query  types.String `tfsdk:"query"`
	Images []imageModel `tfsdk:"images"`
}

type imageModel struct {
	Name        types.String `tfsdk:"name"`
	Aliases     types.List   `tfsdk:"aliases"`
	OS          types.String `tfsdk:"os"`
	Release     types.String `tfsdk:"release"`
	Remote      types.String `tfsdk:"remote"`
	Version     types.String `tfsdk:"version"`
	Description types.String `tfsdk:"description"`
	Kind        types.String `tfsdk:"kind"`
}

func (d *imagesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_images"
}

func (d *imagesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists Multipass images and blueprints available via `multipass find`.",
		Attributes: map[string]schema.Attribute{
			"name": schema.StringAttribute{
				Optional:    true,
				Description: "Exact image name filter.",
			},
			"alias": schema.StringAttribute{
				Optional:    true,
				Description: "Alias filter (matches any alias).",
			},
			"kind": schema.StringAttribute{
				Optional:    true,
				Description: "Filter by kind (`image` or `blueprint`).",
			},
			"query": schema.StringAttribute{
				Optional:    true,
				Description: "Case-insensitive substring filter applied to names and descriptions.",
			},
			"images": schema.ListNestedAttribute{
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Computed: true,
						},
						"aliases": schema.ListAttribute{
							ElementType: types.StringType,
							Computed:    true,
						},
						"os": schema.StringAttribute{
							Computed: true,
						},
						"release": schema.StringAttribute{
							Computed: true,
						},
						"remote": schema.StringAttribute{
							Computed: true,
						},
						"version": schema.StringAttribute{
							Computed: true,
						},
						"description": schema.StringAttribute{
							Computed: true,
						},
						"kind": schema.StringAttribute{
							Computed: true,
						},
					},
				},
			},
		},
	}
}

func (d *imagesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data := req.ProviderData.(providerData)
	d.client = data.client
}

func (d *imagesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.client == nil {
		resp.Diagnostics.AddError("Client not configured", "Multipass client is nil.")
		return
	}

	var config imagesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	images, err := d.client.ListImages(ctx, false)
	if err != nil {
		resp.Diagnostics.AddError("Failed to list images", err.Error())
		return
	}

	filtered := filterImages(images, config)
	model := imagesDataSourceModel{
		Name:   config.Name,
		Alias:  config.Alias,
		Kind:   config.Kind,
		Query:  config.Query,
		Images: flattenImages(ctx, filtered, &resp.Diagnostics),
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func filterImages(images []models.Image, config imagesDataSourceModel) []models.Image {
	var results []models.Image
	name := valueOrEmpty(config.Name)
	alias := valueOrEmpty(config.Alias)
	kind := valueOrEmpty(config.Kind)
	query := strings.ToLower(valueOrEmpty(config.Query))

	for _, img := range images {
		if name != "" && img.Name != name {
			continue
		}
		if alias != "" && !containsIgnoreCase(img.Aliases, alias) {
			continue
		}
		if kind != "" && string(img.Kind) != kind {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(img.Name+" "+img.Description), query) {
			continue
		}
		results = append(results, img)
	}
	return results
}

func flattenImages(ctx context.Context, images []models.Image, diags *diag.Diagnostics) []imageModel {
	result := make([]imageModel, 0, len(images))
	for _, img := range images {
		aliases, diag := types.ListValueFrom(ctx, types.StringType, img.Aliases)
		diags.Append(diag...)
		result = append(result, imageModel{
			Name:        types.StringValue(img.Name),
			Aliases:     aliases,
			OS:          types.StringValue(img.OS),
			Release:     types.StringValue(img.Release),
			Remote:      types.StringValue(img.Remote),
			Version:     types.StringValue(img.Version),
			Description: types.StringValue(img.Description),
			Kind:        types.StringValue(string(img.Kind)),
		})
	}
	return result
}

func containsIgnoreCase(values []string, target string) bool {
	for _, v := range values {
		if strings.EqualFold(v, target) {
			return true
		}
	}
	return false
}
