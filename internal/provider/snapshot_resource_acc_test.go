package provider

import (
	"context"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"

	"github.com/todoroff/terraform-provider-multipass/internal/multipasscli"
)

func TestAccSnapshotResource_basic(t *testing.T) {
	instanceName := randomName()
	snapshotName := "snap1"
	rn := "multipass_snapshot.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckInstanceDestroy,
		Steps: []resource.TestStep{
			// Step 1: create the instance (Running).
			{
				Config: testAccSnapshotConfig_instanceOnly(instanceName),
				Check:  resource.TestCheckResourceAttr("multipass_instance.test", "state", "Running"),
			},
			// Step 2: stop the instance, then create the snapshot.
			{
				PreConfig: func() {
					stopInstance(t, instanceName)
				},
				Config: testAccSnapshotConfig_withSnapshot(instanceName, snapshotName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "instance", instanceName),
					resource.TestCheckResourceAttr(rn, "name", snapshotName),
					resource.TestCheckResourceAttr(rn, "id", instanceName+"."+snapshotName),
				),
			},
			// Import by instance.snapshot.
			{
				ResourceName:                         rn,
				ImportState:                          true,
				ImportStateId:                        instanceName + "." + snapshotName,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "instance",
			},
		},
	})
}

func TestAccSnapshotResource_withComment(t *testing.T) {
	instanceName := randomName()
	snapshotName := "snap-comment"
	rn := "multipass_snapshot.test"

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckInstanceDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccSnapshotConfig_instanceOnly(instanceName),
			},
			{
				PreConfig: func() {
					stopInstance(t, instanceName)
				},
				Config: testAccSnapshotConfig_withComment(instanceName, snapshotName, "test comment"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(rn, "name", snapshotName),
					resource.TestCheckResourceAttr(rn, "comment", "test comment"),
				),
			},
		},
	})
}

// stopInstance stops a Multipass instance via the CLI client.
func stopInstance(t *testing.T, name string) {
	t.Helper()
	ctx := context.Background()
	client, err := multipasscli.NewClient(ctx, multipasscli.Config{})
	if err != nil {
		t.Fatalf("failed to create client to stop instance: %v", err)
	}
	if err := client.StopInstance(ctx, name, false); err != nil {
		t.Fatalf("failed to stop instance %s: %v", name, err)
	}
}

func testAccSnapshotConfig_instanceOnly(name string) string {
	return testProviderConfig + fmt.Sprintf(`
resource "multipass_instance" "test" {
  name = %q
}
`, name)
}

func testAccSnapshotConfig_withSnapshot(instanceName, snapshotName string) string {
	return testProviderConfig + fmt.Sprintf(`
resource "multipass_instance" "test" {
  name = %q
}

resource "multipass_snapshot" "test" {
  instance = multipass_instance.test.name
  name     = %q
}
`, instanceName, snapshotName)
}

func testAccSnapshotConfig_withComment(instanceName, snapshotName, comment string) string {
	return testProviderConfig + fmt.Sprintf(`
resource "multipass_instance" "test" {
  name = %q
}

resource "multipass_snapshot" "test" {
  instance = multipass_instance.test.name
  name     = %q
  comment  = %q
}
`, instanceName, snapshotName, comment)
}
