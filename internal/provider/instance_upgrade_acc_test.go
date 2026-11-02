package provider

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/todoroff/terraform-provider-multipass/internal/models"
)

func TestAccInstanceResource_upgradeFromV220(t *testing.T) {
	for _, tc := range []struct {
		name         string
		cpus         int
		memory, disk string
	}{
		{name: "defaults"},
		{name: "custom allocations", cpus: 2, memory: "2G", disk: "10G"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			name := randomName()
			config := resizeAccConfig(name, "", tc.cpus, tc.memory, tc.disk)
			var originalIdentity []byte
			var originalAllocations models.InstanceResources
			var originalZone string
			originalState := map[string]string{}
			check := func(upgraded bool) resource.TestCheckFunc {
				return func(s *terraform.State) error {
					c := resizeAccClient(t)
					ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
					defer cancel()
					instance, err := c.GetInstance(ctx, name)
					if err != nil {
						return err
					}
					if instance.State != "Running" {
						return fmt.Errorf("instance state %q, want Running", instance.State)
					}
					allocations, err := c.GetInstanceResources(ctx, name)
					if err != nil {
						return err
					}
					identity, err := c.ExecCapture(ctx, name, []string{"cat", "/etc/machine-id", "/proc/sys/kernel/random/boot_id"})
					if err != nil {
						return err
					}
					if len(bytes.Fields(identity)) != 2 {
						return fmt.Errorf("expected machine and boot identities, got %q", identity)
					}
					attrs := s.RootModule().Resources["multipass_instance.test"].Primary.Attributes
					if !upgraded {
						if _, ok := attrs["zone"]; ok {
							return fmt.Errorf("2.2.0 state unexpectedly contains zone")
						}
						originalIdentity, originalAllocations, originalZone = identity, allocations, instance.Zone
						for _, field := range []string{"id", "name", "image", "cpus", "memory", "disk", "resize_policy"} {
							originalState[field] = attrs[field]
						}
						return c.Exec(ctx, name, []string{"cp", "/etc/machine-id", "/home/ubuntu/tf-upgrade-marker"})
					}
					if !bytes.Equal(identity, originalIdentity) {
						return fmt.Errorf("provider upgrade replaced or rebooted the VM")
					}
					if !allocations.EqualValues(originalAllocations) {
						return fmt.Errorf("allocations changed from %+v to %+v", originalAllocations, allocations)
					}
					for field, want := range originalState {
						if attrs[field] != want {
							return fmt.Errorf("state field %s changed from %q to %q", field, want, attrs[field])
						}
					}
					if instance.Zone != originalZone || attrs["zone"] != originalZone {
						return fmt.Errorf("zone: daemon %q, state %q; want %q", instance.Zone, attrs["zone"], originalZone)
					}
					if err := c.Exec(ctx, name, []string{"cmp", "-s", "/etc/machine-id", "/home/ubuntu/tf-upgrade-marker"}); err != nil {
						return fmt.Errorf("guest data was not preserved: %w", err)
					}
					t.Logf("2.2.0 upgrade preserved %s: machine/boot identity, guest marker, allocations, existing state, and zone %q", name, originalZone)
					return nil
				}
			}
			resource.Test(t, resource.TestCase{
				PreCheck:     func() { testAccPreCheck(t) },
				CheckDestroy: testAccCheckInstanceDestroy,
				Steps: []resource.TestStep{
					{
						ExternalProviders: map[string]resource.ExternalProvider{"multipass": {
							Source: "registry.terraform.io/todoroff/multipass", VersionConstraint: "= 2.2.0",
						}},
						Config: config,
						Check:  check(false),
					},
					{
						ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
						Config:                   config,
						ConfigPlanChecks: resource.ConfigPlanChecks{
							PreApply: []plancheck.PlanCheck{plancheck.ExpectEmptyPlan()},
						},
						Check: check(true),
					},
				},
			})
		})
	}
}
