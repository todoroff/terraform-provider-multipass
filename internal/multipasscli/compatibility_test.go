package multipasscli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
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
	if err := c.Exec(ctx, "vm", []string{"command"}); !errors.Is(err, ErrTimeout) {
		t.Fatalf("deadline: %v", err)
	}
	t.Setenv("MULTIPASS_TEST_RESPONSES", `{"info vm --format json":{"Stderr":"instance does not exist","ExitCode":255}}`)
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
