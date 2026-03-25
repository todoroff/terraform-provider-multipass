package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccAliasResource_basic(t *testing.T) {
	instanceName := randomName()
	aliasName := randomName()
	rn := "multipass_alias.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(s *terraform.State) error {
			if err := testAccCheckInstanceDestroy(s); err != nil {
				return err
			}
			return testAccCheckAliasDestroy(s)
		},
		Steps: []resource.TestStep{
			{
				Config: testAccAliasConfig_basic(instanceName, aliasName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "name", aliasName),
					resource.TestCheckResourceAttr(rn, "instance", instanceName),
					resource.TestCheckResourceAttr(rn, "command", "ls"),
				),
			},
			// Import by alias name.
			{
				ResourceName:                         rn,
				ImportState:                          true,
				ImportStateId:                        aliasName,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "name",
				ImportStateVerifyIgnore:              []string{"command"},
			},
		},
	})
}

func TestAccAliasResource_workingDirectory(t *testing.T) {
	instanceName := randomName()
	aliasName := randomName()
	rn := "multipass_alias.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(s *terraform.State) error {
			if err := testAccCheckInstanceDestroy(s); err != nil {
				return err
			}
			return testAccCheckAliasDestroy(s)
		},
		Steps: []resource.TestStep{
			{
				Config: testAccAliasConfig_workDir(instanceName, aliasName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "name", aliasName),
					resource.TestCheckResourceAttr(rn, "command", "ls"),
					resource.TestCheckResourceAttr(rn, "working_directory", "/home/ubuntu"),
				),
			},
		},
	})
}

func testAccAliasConfig_basic(instanceName, aliasName string) string {
	return testProviderConfig + fmt.Sprintf(`
resource "multipass_instance" "test" {
  name = %q
}

resource "multipass_alias" "test" {
  name     = %q
  instance = multipass_instance.test.name
  command  = "ls"
}
`, instanceName, aliasName)
}

func testAccAliasConfig_workDir(instanceName, aliasName string) string {
	return testProviderConfig + fmt.Sprintf(`
resource "multipass_instance" "test" {
  name = %q
}

resource "multipass_alias" "test" {
  name              = %q
  instance          = multipass_instance.test.name
  command           = "ls"
  working_directory = "/home/ubuntu"
}
`, instanceName, aliasName)
}
