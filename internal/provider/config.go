package provider

import (
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/todoroff/terraform-provider-multipass/internal/multipasscli"
)

type providerConfigModel struct {
	MultipassPath  types.String `tfsdk:"multipass_path"`
	CommandTimeout types.Int64  `tfsdk:"command_timeout"`
	DefaultImage   types.String `tfsdk:"default_image"`
}

type providerConfig struct {
	BinaryPath     string
	CommandTimeout int
	DefaultImage   string
}

type providerData struct {
	client       multipasscli.Client
	defaultImage string
}
