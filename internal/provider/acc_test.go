package provider

import (
	"context"
	"crypto/rand"
	"fmt"
	"os/exec"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/todoroff/terraform-provider-multipass/internal/multipasscli"
)

var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"multipass": providerserver.NewProtocol6WithError(New("test")()),
}

// testProviderConfig is prepended to every acceptance test HCL config so that
// OpenTofu (and Terraform) can resolve the provider source correctly.
const testProviderConfig = `
terraform {
  required_providers {
    multipass = {
      source = "registry.terraform.io/todoroff/multipass"
    }
  }
}
`

// testAccPreCheck validates that multipass is available before running
// acceptance tests. Tests are also gated by TF_ACC=1 (enforced by the
// terraform-plugin-testing framework).
func testAccPreCheck(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("multipass"); err != nil {
		t.Skip("multipass not found in PATH, skipping acceptance tests")
	}
}

// randomName generates a unique instance/resource name for acceptance tests.
func randomName() string {
	b := make([]byte, 4)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("failed to generate random name: %v", err))
	}
	return fmt.Sprintf("tf-acc-%x", b)
}

// testAccCheckInstanceDestroy verifies that all multipass_instance resources
// tracked in the Terraform state have been removed from the Multipass daemon.
func testAccCheckInstanceDestroy(s *terraform.State) error {
	ctx := context.Background()
	client, err := multipasscli.NewClient(ctx, multipasscli.Config{})
	if err != nil {
		return fmt.Errorf("creating client for destroy check: %w", err)
	}
	instances, err := client.ListInstances(ctx, true)
	if err != nil {
		return fmt.Errorf("listing instances for destroy check: %w", err)
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "multipass_instance" {
			continue
		}
		name := rs.Primary.Attributes["name"]
		for _, inst := range instances {
			if inst.Name == name {
				return fmt.Errorf("instance %s still exists (state: %s)", name, inst.State)
			}
		}
	}
	return nil
}

// testAccCheckAliasDestroy verifies that all multipass_alias resources tracked
// in the Terraform state have been removed.
func testAccCheckAliasDestroy(s *terraform.State) error {
	ctx := context.Background()
	client, err := multipasscli.NewClient(ctx, multipasscli.Config{})
	if err != nil {
		return nil // can't verify, skip
	}
	aliases, err := client.ListAliases(ctx, true)
	if err != nil {
		return nil // can't verify, skip
	}
	for _, rs := range s.RootModule().Resources {
		if rs.Type != "multipass_alias" {
			continue
		}
		name := rs.Primary.Attributes["name"]
		for _, alias := range aliases {
			if alias.Name == name {
				return fmt.Errorf("alias %s still exists after destroy", name)
			}
		}
	}
	return nil
}
