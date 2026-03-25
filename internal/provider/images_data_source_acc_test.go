package provider

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestAccImagesDataSource_basic(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig + `data "multipass_images" "all" {}`,
				Check:  resource.TestCheckResourceAttrSet("data.multipass_images.all", "images.#"),
			},
		},
	})
}

func TestAccImagesDataSource_filteredByKind(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: testProviderConfig + `data "multipass_images" "imgs" { kind = "image" }`,
				Check:  resource.TestCheckResourceAttrSet("data.multipass_images.imgs", "images.#"),
			},
		},
	})
}
