package multipasscli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/todoroff/terraform-provider-multipass/internal/models"
)

func TestAccDeleteInstancePreservesOtherDeletedInstances(t *testing.T) {
	if os.Getenv("TF_ACC") == "" {
		t.Skip("set TF_ACC=1 to run live VM tests")
	}
	if _, err := exec.LookPath("multipass"); err != nil {
		t.Skip("multipass is not installed")
	}
	ctx := context.Background()
	client, err := NewClient(ctx, Config{Timeout: 600})
	if err != nil {
		t.Fatal(err)
	}
	prefix := fmt.Sprintf("tf-acc-purge-%d", time.Now().UnixNano())
	sentinel, managed := prefix+"-keep", prefix+"-delete"
	for _, name := range []string{sentinel, managed} {
		// Cleanup is always scoped to a name owned by this test, including on failure.
		t.Cleanup(func() {
			if err := client.DeleteInstance(context.Background(), name, true); err != nil && !errors.Is(err, ErrNotFound) {
				t.Errorf("cleanup %s: %v", name, err)
			}
		})
		if err := client.LaunchInstance(ctx, models.LaunchOptions{Name: name, Image: "lts", CPUs: 1, Memory: "1G", Disk: "5G"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := client.DeleteInstance(ctx, sentinel, false); err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteInstance(ctx, managed, true); err != nil {
		t.Fatal(err)
	}
	instances, err := client.ListInstances(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, instance := range instances {
		if instance.Name == managed {
			t.Errorf("managed VM still exists: %s", instance.State)
		}
		if instance.Name == sentinel {
			found = strings.EqualFold(instance.State, "Deleted")
		}
	}
	if !found {
		t.Fatal("unrelated soft-deleted test VM was purged")
	}
}
