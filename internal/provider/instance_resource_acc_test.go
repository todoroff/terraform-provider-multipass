package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/todoroff/terraform-provider-multipass/internal/multipasscli"
)

func TestAccInstanceResource_basic(t *testing.T) {
	name := randomName()
	rn := "multipass_instance.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckInstanceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccInstanceConfig_basic(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "name", name),
					resource.TestCheckResourceAttr(rn, "id", name),
					resource.TestCheckResourceAttr(rn, "state", "Running"),
					resource.TestCheckResourceAttrSet(rn, "release"),
					resource.TestCheckResourceAttrSet(rn, "last_updated"),
				),
			},
			// Import by instance name.
			{
				ResourceName:                         rn,
				ImportState:                          true,
				ImportStateId:                        name,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "name",
				// Import cannot recover the original size spelling; compare sizes
				// numerically instead of ignoring their values entirely.
				ImportStateVerifyIgnore: []string{"image", "last_updated", "memory", "disk"},
				ImportStateCheck: func(states []*terraform.InstanceState) error {
					if len(states) != 1 {
						return fmt.Errorf("expected one imported instance")
					}
					for field, want := range map[string]uint64{"memory": 1 << 30, "disk": 5 << 30} {
						got, err := multipasscli.ParseSize(states[0].Attributes[field])
						if err != nil || got != want {
							return fmt.Errorf("imported %s: %d, %v; want %d", field, got, err, want)
						}
					}
					return nil
				},
			},
		},
	})
}

func TestAccInstanceResource_customSpecs(t *testing.T) {
	name := randomName()
	rn := "multipass_instance.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckInstanceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccInstanceConfig_customSpecs(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "name", name),
					resource.TestCheckResourceAttr(rn, "state", "Running"),
					resource.TestCheckResourceAttrSet(rn, "release"),
				),
			},
		},
	})
}

func TestAccInstanceResource_cloudInit(t *testing.T) {
	name := randomName()
	rn := "multipass_instance.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckInstanceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccInstanceConfig_cloudInit(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "name", name),
					resource.TestCheckResourceAttr(rn, "state", "Running"),
				),
			},
		},
	})
}

func testAccInstanceConfig_basic(name string) string {
	return testProviderConfig + fmt.Sprintf(`
resource "multipass_instance" "test" {
  name = %q
}
`, name)
}

func testAccInstanceConfig_customSpecs(name string) string {
	return testProviderConfig + fmt.Sprintf(`
resource "multipass_instance" "test" {
  name   = %q
  cpus   = 2
  memory = "2G"
  disk   = "10G"
}
`, name)
}

func testAccInstanceConfig_cloudInit(name string) string {
	return testProviderConfig + fmt.Sprintf(`
resource "multipass_instance" "test" {
  name = %q
  cloud_init = <<-YAML
    #cloud-config
    runcmd:
      - echo "acceptance-test" > /tmp/tf-acc-test
  YAML
}
`, name)
}
