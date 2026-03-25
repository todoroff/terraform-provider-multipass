package provider

import (
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
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
