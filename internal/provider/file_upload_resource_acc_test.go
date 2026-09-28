package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
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
  content = "IP=${multipass_instance.test.ipv4[0]}"
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
				want := "IP=" + state.RootModule().Resources["multipass_instance.test"].Primary.Attributes["ipv4.0"]
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
