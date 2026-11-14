package multipasscli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/todoroff/terraform-provider-multipass/internal/models"
)

func TestGuestFailuresPreserveExitStatus(t *testing.T) {
	for _, code := range []int{1, 2, 127, 255} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			c, _ := recordingClient(t)
			t.Setenv("MULTIPASS_TEST_RESPONSES", fmt.Sprintf(`{"exec vm -- command":{"Stdout":"details","Stderr":"not found; timeout; does not exist","ExitCode":%d}}`, code))
			err := c.Exec(context.Background(), "vm", []string{"command"})
			var cliErr *CLIError
			var exitErr *exec.ExitError
			if !errors.As(err, &cliErr) || !errors.As(err, &exitErr) || exitErr.ExitCode() != code || cliErr.Stdout != "details" || cliErr.Stderr != "not found; timeout; does not exist" || errors.Is(err, ErrNotFound) || errors.Is(err, ErrTimeout) {
				t.Fatalf("guest failure lost: %#v", err)
			}
		})
	}
	c, _ := recordingClient(t)
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	if err := c.Exec(ctx, "vm", []string{"command"}); !errors.Is(err, ErrNotStarted) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline: %v", err)
	}
	t.Setenv("MULTIPASS_TEST_RESPONSES", `{"info vm --format json":{"Stderr":"info failed: The following errors occurred:\ninstance \"vm\" does not exist","ExitCode":255}}`)
	if _, err := c.GetInstance(context.Background(), "vm"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("daemon diagnostic: %v", err)
	}
}

func Test117ImageAndMountPayloads(t *testing.T) {
	var catalog findResponse
	if err := json.Unmarshal([]byte(`{"images":{"fedora":{"version":"44","release":"44","remote":"release","os":"Fedora","aliases":[]},"debian":{"aliases":["trixie"],"os":"Debian","release":"13","version":"20261001","remote":"release"}},"errors":[]}`), &catalog); err != nil {
		t.Fatal(err)
	}
	images := catalog.toModel()
	if len(images) != 2 || images[0].Name != "debian" || images[0].OS != "Debian" || images[1].OS != "Fedora" || string(images[1].Kind) != "image" {
		t.Fatalf("images: %+v", images)
	}
	if err := json.Unmarshal([]byte(`{"blueprints (deprecated)":{"legacy":{"release":"Legacy"}}}`), &catalog); err != nil {
		t.Fatal(err)
	}
	if len(catalog.toModel()) != 3 {
		t.Fatal("legacy blueprints lost")
	}
	var info infoResponse
	if err := json.Unmarshal([]byte(`{"info":{"vm":{"state":"Running","mounts":{"/workspace":{"source_path":"/host"}},"cpu_count":"2"}}}`), &info); err != nil {
		t.Fatal(err)
	}
	vm, err := info.toModel("vm")
	if err != nil || vm.CPUCount != 2 || len(vm.Mounts) != 1 || vm.Mounts[0].InstancePath != "/workspace" {
		t.Fatalf("vm: %+v, %v", vm, err)
	}
}

func TestCommandNotFoundDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		args    []string
		stderr  string
		missing bool
	}{
		{[]string{"info", "vm", "--format", "json"}, "info failed: The following errors occurred:\r\ninstance \"vm\" does not exist\r\n", true},
		{[]string{"delete", "--purge", "vm"}, "delete failed: The following errors occurred:\ninstance \"vm\" does not exist", true},
		{[]string{"recover", "vm"}, "recover failed: instance 'vm' does not exist", true},
		{[]string{"delete", "--purge", "vm.snap"}, "delete failed: No such snapshot: vm.snap", true},
		{[]string{"delete", "--purge", "vm.snap"}, "delete failed: The following errors occurred:\ninstance \"vm\" does not exist", true},
		{[]string{"unalias", "default.shell"}, "Nonexistent alias: default.shell.", true},
		{[]string{"get", "--raw", "local.vm.cpus"}, "get failed: Cannot obtain instance settings; instance: vm; reason: No such instance", true},
		{[]string{"set", "local.vm.cpus=2"}, "set failed: Cannot update instance settings; instance: vm; reason: No such instance", true},
		{[]string{"info", "vm"}, "info failed: SSH key not found", false},
		{[]string{"delete", "--purge", "vm"}, "delete failed: backing file does not exist", false},
		{[]string{"info", "vm"}, "info failed: instance \"other\" does not exist", false},
		{[]string{"info", "vm"}, "info failed: The following errors occurred:\ninstance \"vm\" does not exist\npermission denied", false},
		{[]string{"unalias", "default.shell"}, "Nonexistent alias: default.other.", false},
		{[]string{"exec", "vm", "--", "command"}, "instance \"vm\" does not exist", false},
		{[]string{"get", "--raw", "local.vm.cpus"}, "get failed: Cannot obtain instance settings; instance: other; reason: No such instance", false},
	} {
		t.Run(strings.Join(tc.args, " ")+"/"+tc.stderr, func(t *testing.T) {
			c, _ := recordingClient(t)
			response, err := json.Marshal(map[string]any{strings.Join(tc.args, " "): map[string]any{"Stderr": tc.stderr, "ExitCode": 1}})
			if err != nil {
				t.Fatal(err)
			}
			t.Setenv("MULTIPASS_TEST_RESPONSES", string(response))
			_, err = c.run(context.Background(), tc.args...)
			if err == nil || errors.Is(err, ErrNotFound) != tc.missing {
				t.Fatalf("missing=%v; error=%v", tc.missing, err)
			}
			if !tc.missing {
				var cliErr *CLIError
				if !errors.As(err, &cliErr) {
					t.Fatalf("lost original CLI failure: %v", err)
				}
			}
		})
	}
}

// Literal daemon.cpp v1.17.0 diagnostics, before CLI newline normalization.
const hyperVWarning = "*** Warning! The Hyper-V driver is deprecated and will be removed in an upcoming release. ***\n\nWhen you are ready to have your instances migrated, please stop them (multipass stop --all) and switch to the new Host Compute System (HCS) driver (multipass set local.driver=hcs).\n\n"
const virtualBoxWarning = "*** Warning! The VirtualBox driver is deprecated and will be removed in an upcoming release. ***\n\nWe recommend switching to the new Host Compute System (HCS) driver as soon as possible (multipass set local.driver=hcs). Your instances will not be destroyed but they will be unreachable from the new driver. You can switch back to the old driver for now, but you will need to manually recreate any instances you want to keep in the next release.\n\n"

// Literal utils.cpp and daemon.cpp v1.16.1 diagnostics; preserve upstream wording.
const lxdWarning = "*** Warning! The lxd driver is deprecated and will be removed in an future release. ***\n\nInstances will no longer be available in Multipass then, but they will remain in LXD.\n\n"
const libvirtWarning = "*** Warning! The libvirt driver is deprecated and will be removed in an future release. ***\n\nInstances will remain available with the QEMU driver.\n\n"
const blueprintWarning = "*** Warning! Blueprints are deprecated and will be removed in a future release. ***\n\nYou can achieve similar results with cloud-init and other launch options.\nRun `multipass help launch` for more info, or find out more at:\n- https://documentation.ubuntu.com/multipass/en/latest/how-to-guides/manage-instances/launch-customized-instances-with-multipass-and-cloud-init/\n- https://cloudinit.readthedocs.io\n\n"

func TestWarningDiagnostics(t *testing.T) {
	appleWarning := strings.ReplaceAll(strings.ReplaceAll(virtualBoxWarning, "Host Compute System (HCS)", "Apple Virtualization framework"), "local.driver=hcs", "local.driver=applevz")
	for driver, warning := range map[string]string{
		"hyperv": hyperVWarning, "virtualbox-windows": virtualBoxWarning, "virtualbox-macos": appleWarning,
		"lxd": lxdWarning, "libvirt": libvirtWarning, "blueprint": blueprintWarning,
		"lxd-blueprint": lxdWarning + blueprintWarning, "libvirt-blueprint": libvirtWarning + blueprintWarning,
	} {
		for _, diagnostic := range []struct {
			args   []string
			detail string
			want   error
		}{
			{[]string{"info", "vm"}, `info failed: instance "vm" does not exist`, ErrNotFound},
			{[]string{"delete", "--purge", "vm"}, `delete failed: The following errors occurred:
instance "vm" does not exist`, ErrNotFound},
			{[]string{"launch", "--name", "vm", "--timeout", "5"}, `launch failed: instance "vm" already exists`, ErrAlreadyExists},
			{[]string{"launch", "--name", "vm", "--timeout", "5"}, `launch failed: instance "vm" is being prepared`, ErrAlreadyExists},
		} {
			for _, extra := range []string{"", "permission denied\n"} {
				t.Run(driver+"/"+diagnostic.detail+"/"+extra, func(t *testing.T) {
					c, _ := recordingClient(t)
					stderr := strings.ReplaceAll(warning+extra+diagnostic.detail, "\n", "\r\n")
					response, err := json.Marshal(map[string]any{strings.Join(diagnostic.args, " "): map[string]any{"Stderr": stderr, "ExitCode": 1}})
					if err != nil {
						t.Fatal(err)
					}
					t.Setenv("MULTIPASS_TEST_RESPONSES", string(response))
					if diagnostic.want == ErrAlreadyExists {
						err = c.LaunchInstance(context.Background(), models.LaunchOptions{Name: "vm"})
					} else {
						_, err = c.run(context.Background(), diagnostic.args...)
					}
					if err == nil || errors.Is(err, diagnostic.want) != (extra == "") {
						t.Fatalf("classification %v with extra %q: %v", diagnostic.want, extra, err)
					}
				})
			}
		}
	}
	for _, warning := range []string{
		"*** Warning! Unknown warning.\n\n",
		strings.Replace(hyperVWarning, "stop them", "delete them", 1),
		strings.Replace(lxdWarning, "in an future", "in an upcoming", 1),
		lxdWarning + "*** Warning! Unknown warning.\n\n" + blueprintWarning,
		strings.TrimSuffix(blueprintWarning, "- https://cloudinit.readthedocs.io\n\n"),
	} {
		if isNotFoundError([]string{"info", "vm"}, warning+`info failed: instance "vm" does not exist`) {
			t.Fatal("unrecognized warning discarded")
		}
	}
}
