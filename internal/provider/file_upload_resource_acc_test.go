package provider

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/todoroff/terraform-provider-multipass/internal/multipasscli"
)

func TestAccFileUploadResource_content(t *testing.T) {
	instanceName := randomName()
	rn := "multipass_file_upload.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckInstanceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccFileUploadConfig_content(instanceName, "hello from terraform"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "instance", instanceName),
					resource.TestCheckResourceAttr(rn, "destination", "/home/ubuntu/test.txt"),
					resource.TestCheckResourceAttrSet(rn, "content_hash"),
				),
			},
			// Update the content — content_hash should change.
			{
				Config: testAccFileUploadConfig_content(instanceName, "updated content"),
				Check:  resource.TestCheckResourceAttrSet(rn, "content_hash"),
			},
		},
	})
}

func TestAccFileUploadResource_computedContent(t *testing.T) {
	instanceName := randomName()
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckInstanceDestroy,
		Steps: []resource.TestStep{{
			Config: testProviderConfig + fmt.Sprintf(`
resource "multipass_instance" "test" { name = %q }
resource "multipass_file_upload" "test" {
  instance = multipass_instance.test.name
  destination = "/home/ubuntu/computed.env"
  content = "INSTANCE=${multipass_instance.test.id}"
}
`, instanceName),
			Check: func(state *terraform.State) error {
				ctx := context.Background()
				client, err := multipasscli.NewClient(ctx, multipasscli.Config{Timeout: 30})
				if err != nil {
					return err
				}
				data, err := client.TransferCapture(ctx, multipasscli.TransferOptions{Sources: []string{instanceName + ":/home/ubuntu/computed.env"}, Destination: "-"})
				if err != nil {
					return err
				}
				want := "INSTANCE=" + state.RootModule().Resources["multipass_instance.test"].Primary.Attributes["id"]
				if string(data) != want {
					return fmt.Errorf("remote content %q, want %q", data, want)
				}
				return nil
			},
		}},
	})
}

func testAccFileUploadConfig_content(instanceName, content string) string {
	return testProviderConfig + fmt.Sprintf(`
resource "multipass_instance" "test" {
  name = %q
}

resource "multipass_file_upload" "test" {
  instance    = multipass_instance.test.name
  destination = "/home/ubuntu/test.txt"
  content     = %q
}
`, instanceName, content)
}

func TestAccFileUploadResource_upgradeFromV210(t *testing.T) {
	name := randomName()
	source := filepath.Join(t.TempDir(), "bundle")
	updatedSource := filepath.Join(t.TempDir(), "updated-bundle")
	for _, dir := range []string{source, updatedSource} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "payload"), []byte("original"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	base := testProviderConfig + fmt.Sprintf(`
resource "multipass_instance" "test" {
  name = %q
}
`, name)
	upload := func(label, dest, src string) string {
		return fmt.Sprintf(`
resource "multipass_file_upload" %q {
  instance = multipass_instance.test.name
  destination = %q
  source = %q
  recursive = true
}
`, label, dest, filepath.ToSlash(src))
	}
	retained := upload("root", "/home/ubuntu/renamed-root", source) + upload("shared", "/home/ubuntu/shared", source)
	updated := upload("updated", "/home/ubuntu/update-parent", updatedSource)
	checkGuest := func(updatedExists bool) resource.TestCheckFunc {
		return func(*terraform.State) error {
			c := resizeAccClient(t)
			for _, p := range []string{"/home/ubuntu/renamed-root/payload", "/home/ubuntu/shared/bundle/payload", "/home/ubuntu/shared/keep", "/home/ubuntu/update-parent/keep"} {
				if err := c.Exec(context.Background(), name, []string{"test", "-f", p}); err != nil {
					return fmt.Errorf("retained file %s: %w", p, err)
				}
			}
			if updatedExists {
				return c.Exec(context.Background(), name, []string{"grep", "-Fx", "updated", "/home/ubuntu/update-parent/updated-bundle/payload"})
			}
			return c.Exec(context.Background(), name, []string{"test", "!", "-e", "/home/ubuntu/update-parent/updated-bundle"})
		}
	}
	oldProvider := map[string]resource.ExternalProvider{"multipass": {Source: "registry.terraform.io/todoroff/multipass", VersionConstraint: "= 2.1.0"}}
	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) }, CheckDestroy: testAccCheckInstanceDestroy,
		Steps: []resource.TestStep{
			{ExternalProviders: oldProvider, Config: base, Check: func(*terraform.State) error {
				c := resizeAccClient(t)
				if err := c.Exec(context.Background(), name, []string{"mkdir", "-p", "/home/ubuntu/shared", "/home/ubuntu/update-parent"}); err != nil {
					return err
				}
				return c.Exec(context.Background(), name, []string{"touch", "/home/ubuntu/shared/keep", "/home/ubuntu/update-parent/keep"})
			}},
			{ExternalProviders: oldProvider, Config: base + retained + updated},
			{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Config: base + retained + updated, ConfigPlanChecks: resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()}}},
			{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, PreConfig: func() {
				if err := os.WriteFile(filepath.Join(updatedSource, "payload"), []byte("updated"), 0o600); err != nil {
					t.Fatal(err)
				}
			}, Config: base + updated, Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttr("multipass_file_upload.updated", "resolved_destination", "/home/ubuntu/update-parent/updated-bundle"), checkGuest(true),
			)},
			{ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, Config: base, Check: checkGuest(false)},
		},
	})
}
