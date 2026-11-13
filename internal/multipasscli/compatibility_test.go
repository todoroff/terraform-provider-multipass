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
