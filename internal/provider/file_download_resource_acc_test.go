package provider

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestAccFileDownloadResource_updateAndOwnership(t *testing.T) {
	name := randomName()
	dest := t.TempDir()
	sibling := filepath.Join(dest, "keep.txt")
	if err := os.WriteFile(sibling, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	config := func(content string, overwrite bool, trigger string) string {
		return testProviderConfig + fmt.Sprintf(`
resource "multipass_instance" "test" { name = %q }
resource "multipass_file_upload" "test" {
  instance = multipass_instance.test.name
  destination = "/home/ubuntu/download-test.txt"
  content = %q
}

resource "multipass_file_download" "test" {
  instance = multipass_instance.test.name
  source = multipass_file_upload.test.destination
  destination = %q
  overwrite = %t
  triggers = { revision = %q }
  depends_on = [multipass_file_upload.test]
}
`, name, content, filepath.ToSlash(dest), overwrite, trigger)
	}
	check := func(content string) resource.TestCheckFunc {
		return resource.ComposeAggregateTestCheckFunc(
			resource.TestCheckResourceAttr("multipass_file_download.test", "content_hash", hashBytes([]byte(content))),
			func(*terraform.State) error {
				data, err := os.ReadFile(filepath.Join(dest, "download-test.txt"))
				if err != nil {
					return err
				}
				if string(data) != content {
					return fmt.Errorf("downloaded %q, want %q", data, content)
				}
				return nil
			},
		)
	}
	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) }, ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy: func(state *terraform.State) error {
			if err := testAccCheckInstanceDestroy(state); err != nil {
				return err
			}
			if _, err := os.Stat(filepath.Join(dest, "download-test.txt")); !os.IsNotExist(err) {
				return fmt.Errorf("download was not removed: %v", err)
			}
			data, err := os.ReadFile(sibling)
			if err != nil {
				return err
			}
			if string(data) != "keep" {
				return fmt.Errorf("unrelated local file changed")
			}
			return nil
		},
		Steps: []resource.TestStep{
			{Config: config("first", false, "one"), Check: check("first")},
			{Config: config("updated", true, "one"), Check: check("updated")},
			{Config: config("replaced", true, "two"), Check: check("replaced")},
		},
	})
}

func TestAccFileDownloadResource_directory(t *testing.T) {
	name := randomName()
	dest := t.TempDir()
	config := testProviderConfig + fmt.Sprintf(`
resource "multipass_instance" "test" { name = %q }
resource "multipass_file_upload" "test" {
  instance = multipass_instance.test.name
  destination = "/home/ubuntu/download-tree/nested/file.txt"
  content = "directory contents"
}
resource "multipass_file_download" "absolute" {
  instance = multipass_instance.test.name
  source = "/home/ubuntu/download-tree"
  destination = %q
  recursive = true
  depends_on = [multipass_file_upload.test]
}
resource "multipass_file_download" "relative" {
  instance = multipass_instance.test.name
  source = "download-tree"
  destination = %q
  recursive = true
  depends_on = [multipass_file_upload.test]
}
`, name, filepath.ToSlash(filepath.Join(dest, "absolute")), filepath.ToSlash(filepath.Join(dest, "relative")))
	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) }, ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: testAccCheckInstanceDestroy,
		Steps: []resource.TestStep{{Config: config, Check: func(*terraform.State) error {
			for _, subdir := range []string{"absolute", "relative"} {
				data, err := os.ReadFile(filepath.Join(dest, subdir, "nested", "file.txt"))
				if err != nil {
					return err
				}
				if string(data) != "directory contents" {
					return fmt.Errorf("wrong directory contents: %q", data)
				}
			}
			return nil
		}}},
	})
}
