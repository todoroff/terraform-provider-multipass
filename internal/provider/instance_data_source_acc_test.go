package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccInstanceDataSource_basic(t *testing.T) {
	name := randomName()
	dsn := "data.multipass_instance.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckInstanceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccInstanceDataSourceConfig(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(dsn, "name", name),
					resource.TestCheckResourceAttr(dsn, "state", "Running"),
					resource.TestCheckResourceAttrSet(dsn, "release"),
					resource.TestCheckResourceAttrSet(dsn, "cpu_count"),
					resource.TestCheckResourceAttrSet(dsn, "memory_total_bytes"),
					resource.TestCheckResourceAttrSet(dsn, "disk_total_bytes"),
				),
			},
		},
	})
}

func testAccInstanceDataSourceConfig(name string) string {
	return testProviderConfig + fmt.Sprintf(`
resource "multipass_instance" "test" {
  name = %q
}

data "multipass_instance" "test" {
  name = multipass_instance.test.name
}
`, name)
}
