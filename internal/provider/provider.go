package provider

import (
	"context"
	"fmt"
	"sync"

	"github.com/hashicorp/go-version"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/todoroff/terraform-provider-multipass/internal/multipasscli"
)

const (
	defaultBinaryName = "multipass"
	defaultTimeoutSec = 120
)

// New returns a function that instantiates a Multipass provider configured with
// the supplied version string (injected from the main package).
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &MultipassProvider{
			version: version,
		}
	}
}

var _ provider.Provider = (*MultipassProvider)(nil)

// MultipassProvider implements the Terraform Plugin Framework provider.Provider interface.
type MultipassProvider struct {
	version string

	mu     sync.RWMutex
	client multipasscli.Client
}

// Metadata sets the provider type name and version exposed to Terraform.
func (p *MultipassProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "multipass"
	resp.Version = p.version
}

// Schema defines the provider-level configuration attributes.
func (p *MultipassProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Provider for managing Canonical Multipass resources via the Multipass CLI.",
		Attributes: map[string]schema.Attribute{
			"multipass_path": schema.StringAttribute{
				Optional:            true,
				Description:         "Path to the multipass binary. Defaults to the first multipass found in PATH.",
				MarkdownDescription: "Path to the `multipass` binary. Defaults to `multipass`, which requires the CLI to be available on the `PATH`.",
			},
			"command_timeout": schema.Int64Attribute{
				Optional: true,
				Description: fmt.Sprintf(
					"Timeout for multipass commands in seconds (default: %d).",
					defaultTimeoutSec,
				),
			},
			"default_image": schema.StringAttribute{
				Optional:    true,
				Description: "Default image alias or name used when a resource omits an explicit image value.",
			},
		},
	}
}

// Configure builds the Multipass CLI client shared across resources and data sources.
func (p *MultipassProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config providerConfigModel

	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cfg := providerConfig{
		BinaryPath:     defaultBinaryName,
		DefaultImage:   "",
		CommandTimeout: defaultTimeoutSec,
	}

	if !config.MultipassPath.IsNull() && !config.MultipassPath.IsUnknown() {
		cfg.BinaryPath = config.MultipassPath.ValueString()
	}

	if !config.CommandTimeout.IsNull() && !config.CommandTimeout.IsUnknown() {
		if config.CommandTimeout.ValueInt64() <= 0 {
			resp.Diagnostics.AddAttributeError(
				path.Root("command_timeout"),
				"Invalid command timeout",
				"Timeout must be a positive integer representing seconds.",
			)
			return
		}
		cfg.CommandTimeout = int(config.CommandTimeout.ValueInt64())
	}

	if !config.DefaultImage.IsNull() && !config.DefaultImage.IsUnknown() {
		cfg.DefaultImage = config.DefaultImage.ValueString()
	}

	client, err := multipasscli.NewClient(ctx, multipasscli.Config{
		BinaryPath: cfg.BinaryPath,
		Timeout:    cfg.CommandTimeout,
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to create multipass client", err.Error())
		return
	}

	ver, vErr := client.Version(ctx)
	if vErr != nil {
		resp.Diagnostics.AddWarning(
			"Unable to detect multipass version",
			fmt.Sprintf("Multipass client could not report its version: %v", vErr),
		)
	} else {
		if err := ensureSupportedVersion(ver); err != nil {
			resp.Diagnostics.AddWarning("Unsupported multipass version", err.Error())
		} else {
			tflog.Info(ctx, "Detected Multipass CLI", map[string]any{"version": ver})
		}
	}

	p.mu.Lock()
	p.client = client
	p.mu.Unlock()

	resp.ResourceData = providerData{
		client:       client,
		defaultImage: cfg.DefaultImage,
	}
	resp.DataSourceData = resp.ResourceData
}

// Resources returns the list of resources exposed by the provider.
func (p *MultipassProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewInstanceResource,
		NewAliasResource,
	}
}

// DataSources returns the list of data sources supported by the provider.
func (p *MultipassProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{
		NewImagesDataSource,
		NewNetworksDataSource,
		NewInstanceDataSource,
	}
}

func ensureSupportedVersion(raw string) error {
	// Multipass continuously evolves, but we expect at least v1.13.0 for JSON outputs used here.
	min := version.Must(version.NewVersion("1.13.0"))
	current, err := version.NewVersion(raw)
	if err != nil {
		return fmt.Errorf("could not parse multipass version %q: %w", raw, err)
	}

	if current.LessThan(min) {
		return fmt.Errorf("multipass version %s is older than supported minimum %s", current.Original(), min.Original())
	}
	return nil
}
