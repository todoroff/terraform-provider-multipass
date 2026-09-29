package multipasscli

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/todoroff/terraform-provider-multipass/internal/models"
)

func TestParseSize(t *testing.T) {
	for value, want := range map[string]uint64{"1G": 1 << 30, "1024M": 1 << 30, "1.5GiB": 3 << 29, "512m": 512 << 20, "1T": 1 << 40, "1024": 1024, "1B": 1, "1.2KiB": 1228, "010G": 10 << 30, "08M": 8 << 20, "01.01G": 1084479242, "9223372036854775807": 9223372036854775807} {
		got, err := ParseSize(value)
		if err != nil || got != want {
			t.Errorf("ParseSize(%q) = %d, %v; want %d", value, got, err, want)
		}
	}
	for _, value := range []string{"", "0", "0G", "-1G", "1.1B", "1.5", " 1G", "1G\n", "1e9", "NaN", "9223372036854775808", "999999999999999999999T", "1P", "0.0001K"} {
		if _, err := ParseSize(value); err == nil {
			t.Errorf("accepted invalid size %q", value)
		}
	}
}

func TestSizeMatchesRoundedReport(t *testing.T) {
	for _, tc := range []struct {
		known  uint64
		report string
		match  bool
	}{
		{1537 << 20, "1.5GiB", true}, {3 << 30, "3.0GiB", true}, {3 << 30, "2.0GiB", false},
		{1537 << 20, "1610612736", false}, {1 << 40, "1024.0GiB", true},
	} {
		reported, err := ParseSize(tc.report)
		if err != nil {
			t.Fatal(err)
		}
		if got := SizeMatchesReport(tc.known, reported, tc.report); got != tc.match {
			t.Errorf("%d vs %s: %t, want %t", tc.known, tc.report, got, tc.match)
		}
	}
}

func TestInstanceResourceCommands(t *testing.T) {
	c, logPath := recordingClient(t)
	t.Setenv("MULTIPASS_TEST_RESPONSES", `{"get --raw local.vm.cpus":{"Stdout":"2\r\n"},"get --raw local.vm.memory":{"Stdout":"\u001b[0m2GiB\r\n"},"get --raw local.vm.disk":{"Stdout":"10737418240\n"}}`)
	got, err := c.GetInstanceResources(context.Background(), "vm")
	want := models.InstanceResources{CPUs: 2, MemoryBytes: 2 << 30, DiskBytes: 10 << 30}
	if err != nil || !got.EqualValues(want) {
		t.Fatalf("got %+v, %v", got, err)
	}
	c.instanceCache = newCacheEntry([]models.Instance{{Name: "vm"}}, time.Minute)
	for _, setting := range []ResourceSetting{ResourceCPUs, ResourceMemory, ResourceDisk} {
		if err := c.SetInstanceResource(context.Background(), "vm", setting, 4); err != nil {
			t.Fatal(err)
		}
	}
	if c.instanceCache.valid(time.Now()) {
		t.Fatal("allocation change did not invalidate cache")
	}
	f, err := os.Open(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	decoder := json.NewDecoder(f)
	for _, expected := range [][]string{{"get", "--raw", "local.vm.cpus"}, {"get", "--raw", "local.vm.memory"}, {"get", "--raw", "local.vm.disk"}, {"set", "local.vm.cpus=4"}, {"set", "local.vm.memory=4"}, {"set", "local.vm.disk=4"}} {
		var command []string
		if err := decoder.Decode(&command); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(command, expected) {
			t.Fatalf("command %v, want %v", command, expected)
		}
	}
}

func TestInvalidResourceSettingsNeverInvokeCLI(t *testing.T) {
	c, logPath := recordingClient(t)
	for _, tc := range []struct {
		name    string
		setting ResourceSetting
		value   uint64
	}{{"", ResourceCPUs, 1}, {"vm", "bridged", 1}, {"vm", ResourceDisk, 0}, {"vm", ResourceMemory, 1 << 63}} {
		if err := c.SetInstanceResource(context.Background(), tc.name, tc.setting, tc.value); err == nil {
			t.Fatal("invalid setting accepted")
		}
	}
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Fatal("invalid setting invoked CLI")
	}
}

func TestReadAllocationErrors(t *testing.T) {
	for _, response := range []string{`{"Stdout":"garbage"}`, `{"Stdout":"0"}`, `{"Stderr":"backend unavailable","ExitCode":1}`} {
		c, _ := recordingClient(t)
		t.Setenv("MULTIPASS_TEST_RESPONSES", `{"get --raw local.vm.cpus":`+response+`}`)
		if _, err := c.GetInstanceResources(context.Background(), "vm"); err == nil {
			t.Fatal("bad allocation response accepted")
		}
	}
}
