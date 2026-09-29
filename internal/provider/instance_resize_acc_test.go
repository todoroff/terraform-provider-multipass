package provider

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/todoroff/terraform-provider-multipass/internal/models"
	"github.com/todoroff/terraform-provider-multipass/internal/multipasscli"
)

func resizeAccConfig(name, policy string, cpus int, memory, disk string) string {
	sizes := ""
	if cpus > 0 {
		sizes = fmt.Sprintf("cpus = %d\nmemory = %q\ndisk = %q\n", cpus, memory, disk)
	}
	p := ""
	if policy != "" {
		p = fmt.Sprintf("resize_policy = %q\n", policy)
	}
	return testProviderConfig + fmt.Sprintf(`
resource "multipass_instance" "test" {
  name = %q
  image = "24.04"
  %s
  %s
  timeouts { create = "10m" }
}
`, name, sizes, p)
}

func resizeAccClient(t *testing.T) multipasscli.Client {
	t.Helper()
	c, err := multipasscli.NewClient(context.Background(), multipasscli.Config{Timeout: 120})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func stopResizeAccVM(t *testing.T, name string) {
	t.Helper()
	c := resizeAccClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := c.StopInstance(ctx, name, false); err != nil {
		t.Fatal(err)
	}
	// On VirtualBox, stop can return as soon as the ACPI request is sent.
	// Confirm shutdown before presenting Terraform with a stopped instance.
	r := &instanceResource{client: c}
	if err := r.waitForPowerState(ctx, name, "Stopped"); err != nil {
		t.Fatal(err)
	}
}

func resizePlanCheck(action plancheck.ResourceActionType) resource.ConfigPlanChecks {
	return resource.ConfigPlanChecks{PreApply: []plancheck.PlanCheck{plancheck.ExpectResourceAction("multipass_instance.test", action)}}
}

func TestAccInstanceResource_resizeInPlace(t *testing.T) {
	name := randomName()
	var machineID []byte
	check := func(cpus int64, memory, disk uint64, power string) resource.TestCheckFunc {
		return func(*terraform.State) error {
			c := resizeAccClient(t)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			got, err := c.GetInstanceResources(ctx, name)
			if err != nil {
				return err
			}
			want := models.InstanceResources{CPUs: cpus, MemoryBytes: memory, DiskBytes: disk}
			if !got.EqualValues(want) {
				return fmt.Errorf("allocations %+v, want %+v", got, want)
			}
			instances, err := c.ListInstances(ctx, true)
			if err != nil {
				return err
			}
			found := false
			for _, vm := range instances {
				if vm.Name == name {
					found = true
					if vm.State != power {
						return fmt.Errorf("power %s, want %s", vm.State, power)
					}
				}
			}
			if !found {
				return fmt.Errorf("missing test VM %s", name)
			}
			// Do not exec or transfer while stopped: those operations can start a VM.
			if power == "Stopped" {
				return nil
			}
			identity, err := c.TransferCapture(ctx, multipasscli.TransferOptions{Sources: []string{name + ":/etc/machine-id"}, Destination: "-"})
			if err != nil {
				return err
			}
			if machineID == nil {
				machineID = identity
				return c.Exec(ctx, name, []string{"sh", "-c", "cp /etc/machine-id /home/ubuntu/tf-resize-marker"})
			}
			if !bytes.Equal(machineID, identity) {
				return fmt.Errorf("in-place resize replaced machine identity")
			}
			return c.Exec(ctx, name, []string{"cmp", "-s", "/etc/machine-id", "/home/ubuntu/tf-resize-marker"})
		}
	}
	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) }, ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: testAccCheckInstanceDestroy,
		Steps: []resource.TestStep{
			{Config: resizeAccConfig(name, "", 1, "1G", "8G"), Check: check(1, 1<<30, 8<<30, "Running")},
			{Config: resizeAccConfig(name, "", 2, "2G", "10G"), ConfigPlanChecks: resizePlanCheck(plancheck.ResourceActionUpdate), Check: check(2, 2<<30, 10<<30, "Running")},
			{Config: resizeAccConfig(name, "", 2, "2G", "8G"), ExpectError: regexp.MustCompile("Disk shrink is not supported")},
			{Config: resizeAccConfig(name, "", 1, "1G", "10G"), ConfigPlanChecks: resizePlanCheck(plancheck.ResourceActionUpdate), Check: check(1, 1<<30, 10<<30, "Running")},
			{PreConfig: func() {
				stopResizeAccVM(t, name)
			}, Config: resizeAccConfig(name, "", 2, "2G", "10G"), ConfigPlanChecks: resizePlanCheck(plancheck.ResourceActionUpdate), Check: check(2, 2<<30, 10<<30, "Stopped")},
			{Config: resizeAccConfig(name, "", 0, "", ""), Check: check(2, 2<<30, 10<<30, "Stopped")},
			{Config: resizeAccConfig(name, "", 0, "", ""), PlanOnly: true},
			{Config: resizeAccConfig(name, "replace", 2, "2048M", "10240M"), ConfigPlanChecks: resizePlanCheck(plancheck.ResourceActionUpdate), Check: check(2, 2<<30, 10<<30, "Stopped")},
		},
	})
}

func TestAccInstanceResource_resizeReplace(t *testing.T) {
	name := randomName()
	var identity []byte
	check := func(first bool, cpus int64, memory, disk uint64) resource.TestCheckFunc {
		return func(*terraform.State) error {
			c := resizeAccClient(t)
			got, err := c.GetInstanceResources(context.Background(), name)
			if err != nil {
				return err
			}
			if !got.EqualValues(models.InstanceResources{CPUs: cpus, MemoryBytes: memory, DiskBytes: disk}) {
				return fmt.Errorf("unexpected allocations %+v", got)
			}
			id, err := c.TransferCapture(context.Background(), multipasscli.TransferOptions{Sources: []string{name + ":/etc/machine-id"}, Destination: "-"})
			if err != nil {
				return err
			}
			if first {
				identity = id
				return c.Exec(context.Background(), name, []string{"touch", "/home/ubuntu/tf-resize-marker"})
			}
			if bytes.Equal(identity, id) {
				return fmt.Errorf("replacement retained the old machine identity")
			}
			identity = id
			return c.Exec(context.Background(), name, []string{"test", "!", "-e", "/home/ubuntu/tf-resize-marker"})
		}
	}
	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) }, ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: testAccCheckInstanceDestroy,
		Steps: []resource.TestStep{
			{Config: resizeAccConfig(name, "replace", 1, "1G", "8G"), Check: check(true, 1, 1<<30, 8<<30)},
			{Config: resizeAccConfig(name, "replace", 2, "2G", "10G"), ConfigPlanChecks: resizePlanCheck(plancheck.ResourceActionDestroyBeforeCreate), Check: check(false, 2, 2<<30, 10<<30)},
			{Config: resizeAccConfig(name, "replace", 1, "1G", "8G"), ConfigPlanChecks: resizePlanCheck(plancheck.ResourceActionDestroyBeforeCreate), Check: check(false, 1, 1<<30, 8<<30)},
			{Config: resizeAccConfig(name, "replace", 1, "1G", "8G"), PlanOnly: true},
		},
	})
}

// Keep stopped-instance coverage independent of the running-instance restart
// test, so a host's SSH/startup failure cannot hide stopped resize regressions.
func TestAccInstanceResource_resizeStopped(t *testing.T) {
	name := randomName()
	check := func(cpus int64, memory, disk uint64) resource.TestCheckFunc {
		return func(*terraform.State) error {
			c := resizeAccClient(t)
			actual, err := c.GetInstanceResources(context.Background(), name)
			if err != nil {
				return err
			}
			if !actual.EqualValues(models.InstanceResources{CPUs: cpus, MemoryBytes: memory, DiskBytes: disk}) {
				return fmt.Errorf("unexpected stopped allocations: %+v", actual)
			}
			instances, err := c.ListInstances(context.Background(), true)
			if err != nil {
				return err
			}
			for _, vm := range instances {
				if vm.Name == name {
					if vm.State != "Stopped" {
						return fmt.Errorf("stopped VM was started: %s", vm.State)
					}
					return nil
				}
			}
			return fmt.Errorf("test instance disappeared")
		}
	}
	resource.Test(t, resource.TestCase{
		PreCheck: func() { testAccPreCheck(t) }, ProtoV6ProviderFactories: testAccProtoV6ProviderFactories, CheckDestroy: testAccCheckInstanceDestroy,
		Steps: []resource.TestStep{
			{Config: resizeAccConfig(name, "", 1, "1G", "8G")},
			{PreConfig: func() {
				stopResizeAccVM(t, name)
			}, Config: resizeAccConfig(name, "", 2, "2G", "10G"), ConfigPlanChecks: resizePlanCheck(plancheck.ResourceActionUpdate), Check: check(2, 2<<30, 10<<30)},
			{Config: resizeAccConfig(name, "", 2, "2G", "8G"), ExpectError: regexp.MustCompile("Disk shrink is not supported")},
			{Config: resizeAccConfig(name, "", 1, "1G", "10G"), ConfigPlanChecks: resizePlanCheck(plancheck.ResourceActionUpdate), Check: check(1, 1<<30, 10<<30)},
			{Config: resizeAccConfig(name, "", 0, "", ""), Check: check(1, 1<<30, 10<<30)},
			{Config: resizeAccConfig(name, "replace", 0, "", ""), ConfigPlanChecks: resizePlanCheck(plancheck.ResourceActionUpdate), Check: check(1, 1<<30, 10<<30)},
			{Config: resizeAccConfig(name, "replace", 0, "", ""), PlanOnly: true},
		},
	})
}
