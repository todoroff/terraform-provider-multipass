package provider

import (
	"context"
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/todoroff/terraform-provider-multipass/internal/multipasscli"
)

func TestAccInstanceResource_replacement(t *testing.T) {
	first, second := randomName(), randomName()
	config := func(name, image string) string {
		return testProviderConfig + fmt.Sprintf(`
resource "multipass_instance" "test" {
  name = %q
  image = %q
}
`, name, image)
	}
	check := func(name string) resource.TestCheckFunc {
		return resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr("multipass_instance.test", "id", name),
			func(*terraform.State) error {
				if name == first {
					return nil
				}
				client, err := multipasscli.NewClient(context.Background(), multipasscli.Config{})
				if err != nil {
					return err
				}
				instances, err := client.ListInstances(context.Background(), true)
				if err != nil {
					return err
				}
				for _, instance := range instances {
					if instance.Name == first {
						return fmt.Errorf("old VM still exists after name replacement")
					}
				}
				return nil
			},
		)
	}
	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) }, ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: testAccCheckInstanceDestroy,
		Steps: []resource.TestStep{
			{Config: config(first, "24.04"), Check: check(first)},
			{Config: config(second, "24.04"), Check: check(second)},
			{Config: config(second, "22.04"), Check: resource.ComposeAggregateTestCheckFunc(check(second), func(*terraform.State) error {
				client, err := multipasscli.NewClient(context.Background(), multipasscli.Config{Timeout: 30})
				if err != nil {
					return err
				}
				return client.Exec(context.Background(), second, []string{"grep", "-Fx", `VERSION_ID="22.04"`, "/etc/os-release"})
			})},
		},
	})
}

func TestAccInstanceResource_cloudInitFailure(t *testing.T) {
	name := randomName()
	config := func(exitCode int) string {
		return testProviderConfig + fmt.Sprintf(`
resource "multipass_instance" "test" {
  name = %q
  wait_for_cloud_init = true
  cloud_init = <<-YAML
    #cloud-config
    timezone: Etc/UTC
    runcmd:
      - [sh, -c, "exit %d"]
  YAML
}
resource "multipass_file_upload" "dependent" {
  instance = multipass_instance.test.name
  destination = "/home/ubuntu/dependent.txt"
  content = "only after cloud-init succeeds"
}
`, name, exitCode)
	}
	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) }, ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: testAccCheckInstanceDestroy,
		Steps: []resource.TestStep{
			{Config: config(1), ExpectError: regexp.MustCompile("cloud-init wait failed")},
			{PreConfig: func() {
				client, err := multipasscli.NewClient(context.Background(), multipasscli.Config{Timeout: 30})
				if err != nil {
					t.Fatal(err)
				}
				if err := client.Exec(context.Background(), name, []string{"test", "!", "-e", "/home/ubuntu/dependent.txt"}); err != nil {
					t.Fatalf("dependent upload ran after cloud-init failure: %v", err)
				}
			}, Config: config(0), Check: resource.TestCheckResourceAttrSet("multipass_file_upload.dependent", "id")},
		},
	})
}
