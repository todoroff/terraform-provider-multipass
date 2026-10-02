package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

func TestMain(m *testing.M) {
	if os.Getenv("MULTIPASS_PROVIDER_TEST_CLI") == "1" {
		if len(os.Args) > 1 {
			switch os.Args[1] {
			case "version":
				fmt.Fprint(os.Stdout, `{"multipass":"1.16.0"}`)
				os.Exit(0)
			case "list":
				// Each daemon reports its address as an instance name. This also
				// detects provider clients accidentally sharing cached results.
				_ = json.NewEncoder(os.Stdout).Encode(map[string]any{
					"list": []map[string]string{{"name": os.Getenv("MULTIPASS_SERVER_ADDRESS"), "state": "Running"}},
				})
				os.Exit(0)
			}
		}
		os.Exit(2)
	}
	os.Exit(m.Run())
}

func providerTestConfig(t *testing.T, p provider.Provider, values map[string]any) tfsdk.Config {
	t.Helper()
	var resp provider.SchemaResponse
	p.Schema(context.Background(), provider.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatal(resp.Diagnostics)
	}
	typ := resp.Schema.Type().TerraformType(context.Background()).(tftypes.Object)
	attrs := make(map[string]tftypes.Value, len(typ.AttributeTypes))
	for name, attrType := range typ.AttributeTypes {
		attrs[name] = tftypes.NewValue(attrType, values[name])
	}
	return tfsdk.Config{Schema: resp.Schema, Raw: tftypes.NewValue(typ, attrs)}
}

func TestProviderServerAddressValidation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		value   any
		invalid bool
	}{
		{"omitted", nil, false},
		{"unknown", tftypes.UnknownValue, false},
		{"hostname", "host.example:50051", false},
		{"ipv4", "192.0.2.10:50051", false},
		{"ipv6", "[2001:db8::10]:50051", false},
		{"unix socket", "unix:/run/multipass_socket", false},
		{"empty", "", true},
		{"blank", " ", true},
		{"leading space", " host:50051", true},
		{"newline", "host:50051\n", true},
		{"vertical tab", "host:\v50051", true},
		{"non-breaking space", "host:\u00a050051", true},
		{"next line", "host:\u008550051", true},
		{"unicode line separator", "host:\u202850051", true},
		{"ideographic space", "host:\u300050051", true},
		{"nul", "host\x00:50051", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := New("test")()
			config := providerTestConfig(t, p, map[string]any{"server_address": tc.value})
			server := providerserver.NewProtocol6(p)()
			resp, err := server.ValidateProviderConfig(context.Background(), &tfprotov6.ValidateProviderConfigRequest{Config: dynamicValue(t, config.Raw)})
			if err != nil {
				t.Fatal(err)
			}
			hasError := false
			for _, d := range resp.Diagnostics {
				hasError = hasError || d.Severity == tfprotov6.DiagnosticSeverityError
			}
			if hasError != tc.invalid {
				t.Fatalf("diagnostics = %v, want invalid=%t", resp.Diagnostics, tc.invalid)
			}
		})
	}
}

func TestProviderUnknownServerAddress(t *testing.T) {
	p := New("test")()
	config := providerTestConfig(t, p, map[string]any{"server_address": tftypes.UnknownValue})
	var resp provider.ConfigureResponse
	p.Configure(context.Background(), provider.ConfigureRequest{Config: config}, &resp)
	if len(resp.Diagnostics) != 1 || resp.Diagnostics[0].Summary() != "Unknown Multipass server address" {
		t.Fatalf("unexpected diagnostics: %v", resp.Diagnostics)
	}
	if resp.ResourceData != nil || resp.DataSourceData != nil {
		t.Fatal("unknown address must not configure a client against a fallback daemon")
	}
}

func TestProviderServerAddressConfiguration(t *testing.T) {
	t.Setenv("MULTIPASS_PROVIDER_TEST_CLI", "1")
	t.Setenv("MULTIPASS_SERVER_ADDRESS", "inherited.example:50051")
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	responses := make(map[string]provider.ConfigureResponse)
	for _, address := range []string{"first.example:50051", "second.example:50051", ""} {
		p := New("test")()
		values := map[string]any{"multipass_path": binary, "command_timeout": int64(10)}
		want := address
		if address != "" {
			values["server_address"] = address
		} else {
			want = "inherited.example:50051"
		}
		config := providerTestConfig(t, p, values)
		var resp provider.ConfigureResponse
		p.Configure(context.Background(), provider.ConfigureRequest{Config: config}, &resp)
		if len(resp.Diagnostics) != 0 {
			t.Fatalf("configure %q: %v", address, resp.Diagnostics)
		}
		responses[want] = resp
	}
	for want, resp := range responses {
		t.Run(want, func(t *testing.T) {
			for _, configured := range []any{resp.ResourceData, resp.DataSourceData} {
				data, ok := configured.(providerData)
				if !ok || data.client == nil {
					t.Fatal("missing configured provider data")
				}
				instances, err := data.client.ListInstances(context.Background(), false)
				if err != nil {
					t.Fatal(err)
				}
				if len(instances) != 1 || instances[0].Name != want {
					t.Fatalf("instances = %+v, want server %q", instances, want)
				}
			}
		})
	}
	if got := os.Getenv("MULTIPASS_SERVER_ADDRESS"); got != "inherited.example:50051" {
		t.Fatalf("parent environment changed to %q", got)
	}
}
