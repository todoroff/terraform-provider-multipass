package provider

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/todoroff/terraform-provider-multipass/internal/multipasscli"
)

func testAcc117PreCheck(t *testing.T) {
	t.Helper()
	testAccPreCheck(t)
	c, err := multipasscli.NewClient(context.Background(), multipasscli.Config{Timeout: 30})
	if err != nil {
		t.Fatal(err)
	}
	v, err := c.Version(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := multipasscli.RequireZones(v); err != nil {
		t.Skip(err)
	}
}

func TestAcc117ZonesDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAcc117PreCheck(t) }, ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{{
			Config: testProviderConfig + `
data "multipass_zones" "all" {}
data "multipass_zones" "selected" { name = data.multipass_zones.all.zones[0].name }
data "multipass_zones" "missing" { name = "terraform-nonexistent-zone" }
`,
			Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttrSet("data.multipass_zones.all", "zones.0.name"),
				resource.TestCheckResourceAttrSet("data.multipass_zones.all", "zones.0.subnet"),
				resource.TestCheckResourceAttr("data.multipass_zones.selected", "zones.#", "1"),
				resource.TestCheckResourceAttr("data.multipass_zones.missing", "zones.#", "0"),
				resource.TestCheckResourceAttrPair("data.multipass_zones.all", "zones.0.name", "data.multipass_zones.selected", "zones.0.name"),
				resource.TestCheckResourceAttrPair("data.multipass_zones.all", "zones.0.available", "data.multipass_zones.selected", "zones.0.available"),
				resource.TestCheckResourceAttrPair("data.multipass_zones.all", "zones.0.subnet", "data.multipass_zones.selected", "zones.0.subnet"),
			),
		}},
	})
}

func TestAcc117Zones(t *testing.T) {
	name, automatic := randomName(), randomName()
	config := testProviderConfig + fmt.Sprintf(`
data "multipass_zones" "all" {}
resource "multipass_instance" "test" {
  name = %q
  zone = [for z in data.multipass_zones.all.zones : z.name if z.available][0]
}
resource "multipass_instance" "automatic" { name = %q }
data "multipass_instance" "test" { name = multipass_instance.test.name }
data "multipass_zones" "selected" { name = multipass_instance.test.zone }
`, name, automatic)
	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAcc117PreCheck(t) }, ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: testAccCheckInstanceDestroy,
		Steps: []resource.TestStep{
			{Config: config, Check: resource.ComposeAggregateTestCheckFunc(
				resource.TestCheckResourceAttrSet("multipass_instance.automatic", "zone"),
				resource.TestCheckResourceAttrPair("multipass_instance.test", "zone", "data.multipass_instance.test", "zone"),
				resource.TestCheckResourceAttrPair("multipass_instance.test", "zone", "data.multipass_zones.selected", "zones.0.name"),
				resource.TestCheckResourceAttr("data.multipass_zones.selected", "zones.#", "1"),
			)},
			{ResourceName: "multipass_instance.test", ImportState: true, ImportStateId: name, ImportStateVerify: true, ImportStateVerifyIdentifierAttribute: "name", ImportStateVerifyIgnore: []string{"last_updated", "memory", "disk"}},
		},
	})
}

func TestAcc117Images(t *testing.T) {
	for _, image := range []string{"debian", "fedora"} {
		t.Run(image, func(t *testing.T) {
			name := randomName()
			config := testProviderConfig + fmt.Sprintf(`
resource "multipass_instance" "test" {
  name = %q
  image = %q
  memory = "2G"
  disk = "10G"
  wait_for_cloud_init = true
  timeouts { create = "20m" }
}
resource "multipass_file_upload" "ready" {
  instance = multipass_instance.test.name
  destination = "/tmp/terraform-cloud-init-ready"
  content = "ready"
}
`, name, image)
			resource.Test(t, resource.TestCase{PreCheck: func() { testAcc117PreCheck(t) }, ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: testAccCheckInstanceDestroy, Steps: []resource.TestStep{
				{Config: config, Check: func(*terraform.State) error {
					c, err := multipasscli.NewClient(context.Background(), multipasscli.Config{Timeout: 30})
					if err != nil {
						return err
					}
					return testAccReadOnlyExec(context.Background(), c, name, []string{"sh", "-c", `. /etc/os-release; test "$ID" = "$1" && test "$(cat /tmp/terraform-cloud-init-ready)" = ready`, "sh", image})
				}},
			}})
		})
	}
}

func TestAcc117CloudInitReboot(t *testing.T) {
	name := randomName()
	config := testProviderConfig + fmt.Sprintf(`
resource "multipass_instance" "test" {
  name = %q
  wait_for_cloud_init = true
  cloud_init = <<-YAML
    #cloud-config
    runcmd:
      - [sh, -c, "cat /proc/sys/kernel/random/boot_id > /var/tmp/terraform-first-boot"]
    power_state:
      mode: reboot
      delay: now
      timeout: 120
      condition: true
  YAML
  timeouts { create = "20m" }
}
`, name)
	resource.Test(t, resource.TestCase{PreCheck: func() { testAcc117PreCheck(t) }, ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: testAccCheckInstanceDestroy, Steps: []resource.TestStep{
		{Config: config, Check: func(*terraform.State) error {
			c, err := multipasscli.NewClient(context.Background(), multipasscli.Config{Timeout: 30})
			if err != nil {
				return err
			}
			return testAccReadOnlyExec(context.Background(), c, name, []string{"sh", "-c", `test -s /var/tmp/terraform-first-boot && test "$(cat /proc/sys/kernel/random/boot_id)" != "$(cat /var/tmp/terraform-first-boot)"`})
		}},
	}})
}

// These assertions only read guest state, so reconnecting cannot repeat a mutation.
func testAccReadOnlyExec(ctx context.Context, c multipasscli.Client, name string, command []string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := c.Exec(ctx, name, command)
		var cliErr *multipasscli.CLIError
		var exitErr *exec.ExitError
		timeout := errors.As(err, &cliErr) && errors.As(err, &exitErr) && exitErr.ExitCode() == 255 && strings.HasPrefix(cliErr.Stderr, "exec failed: ssh connection failed: 'Timeout connecting to ")
		if !timeout && !isCloudInitSSHDisconnect(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%w; last SSH failure: %v", ctx.Err(), err)
		case <-time.After(time.Second):
		}
	}
}
