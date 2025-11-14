package multipasscli

import (
	"encoding/json"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestListResponseToModel(t *testing.T) {
	payload := []byte(`{
		"list": [
			{"name":"primary","state":"Running","release":"Ubuntu","ipv4":["10.0.0.2","N/A"]},
			{"name":"dev","state":"Stopped","release":"Ubuntu 24.04","ipv4":[]}
		]
	}`)

	var resp listResponse
	if err := json.Unmarshal(payload, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	models := resp.toModel()
	if len(models) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(models))
	}

	if diff := cmp.Diff([]string{"10.0.0.2"}, models[0].IPv4); diff != "" {
		t.Fatalf("unexpected ipv4 diff: %s", diff)
	}
}

func TestInfoResponseToModel(t *testing.T) {
	payload := []byte(`{
		"info":{
			"primary":{
				"cpu_count":"2",
				"disks":{"sda1":{"total":"1024","used":"512"}},
				"image_hash":"abc",
				"image_release":"24.04",
				"ipv4":["1.1.1.1"],
				"load":[0.1,0.2,0.3],
				"memory":{"total":2048,"used":1024},
				"mounts":{"/data":{"source_path":"/host","readonly":false}},
				"release":"Ubuntu",
				"snapshot_count":"1",
				"state":"Running"
			}
		}
	}`)

	var resp infoResponse
	if err := json.Unmarshal(payload, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	model, err := resp.toModel("primary")
	if err != nil {
		t.Fatalf("toModel: %v", err)
	}

	if model.CPUCount != 2 {
		t.Fatalf("cpu count mismatch: %d", model.CPUCount)
	}
	if len(model.Mounts) != 1 || model.Mounts[0].InstancePath != "/data" {
		t.Fatalf("unexpected mounts: %#v", model.Mounts)
	}
}
