package multipasscli

import (
	"context"
	"encoding/json"
	"github.com/todoroff/terraform-provider-multipass/internal/models"
	"os"
	"strings"
	"testing"
)

func TestZoneVersions(t *testing.T) {
	for _, tc := range []struct {
		client, daemon string
		ok             bool
	}{
		{"1.17.0+win", "1.17.0", true}, {"1.18.0", "1.17.1", true}, {"1.16.1", "1.17.0", false}, {"1.17.0", "1.16.1", false}, {"1.17.0", "", false}, {"bad", "1.17.0", false}, {"1.17.0-rc1", "1.17.0", false},
	} {
		if got := RequireZones(models.VersionInfo{Client: tc.client, Daemon: tc.daemon}); (got == nil) != tc.ok {
			t.Errorf("%+v: %v", tc, got)
		}
	}
	c, _ := recordingClient(t)
	t.Setenv("MULTIPASS_TEST_RESPONSES", `{"version --format json":{"Stdout":"{\"multipass\":\"1.17.0+win\",\"multipassd\":\"1.16.1\"}"}}`)
	v, err := c.Version(context.Background())
	if err != nil || v.Client != "1.17.0+win" || v.Daemon != "1.16.1" {
		t.Fatalf("%+v %v", v, err)
	}
}

func TestZoneCLI(t *testing.T) {
	c, log := recordingClient(t)
	t.Setenv("MULTIPASS_TEST_RESPONSES", `{"zones --format json":{"Stdout":"{\"zone2\":{\"available\":false,\"subnet\":\"10.0.2.0/24\"},\"zone1\":{\"subnet\":\"10.0.1.0/24\",\"available\":true}}"}}`)
	zones, err := c.ListZones(context.Background())
	if err != nil || len(zones) != 2 || zones[0].Name != "zone1" || !zones[0].Available || zones[1].Available || zones[1].Subnet != "10.0.2.0/24" {
		t.Fatalf("%+v %v", zones, err)
	}
	for _, zone := range []string{"zone2", ""} {
		if err := c.LaunchInstance(context.Background(), models.LaunchOptions{Name: "vm", Zone: zone}); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(data), `"--zone","zone2"`) != 1 || strings.Count(string(data), `"--zone"`) != 1 {
		t.Fatal(string(data))
	}
}

func TestZoneInstancePayloads(t *testing.T) {
	for _, field := range []string{`,"zone":{"name":"zone2","available":false}`, `,"zone":null`, ""} {
		var list listResponse
		var info infoResponse
		entry := `{"name":"vm","state":"Unavailable"` + field + `}`
		if err := json.Unmarshal([]byte(`{"list":[`+entry+`]}`), &list); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(`{"info":{"vm":`+entry+`}}`), &info); err != nil {
			t.Fatal(err)
		}
		want := ""
		if strings.Contains(field, "zone2") {
			want = "zone2"
		}
		vm, err := info.toModel("vm")
		if err != nil || vm.Zone != want || list.toModel()[0].Zone != want || vm.State != "Unavailable" {
			t.Fatalf("%+v %v", vm, err)
		}
	}
}
