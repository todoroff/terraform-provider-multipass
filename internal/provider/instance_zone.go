package provider

import (
	"context"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/todoroff/terraform-provider-multipass/internal/models"
	"github.com/todoroff/terraform-provider-multipass/internal/multipasscli"
	"regexp"
	"strings"
)

var zoneNamePattern = regexp.MustCompile(`^[^\s\p{Z}\x00\x0b\x85]+$`)

func nullableZone(zone string) types.String {
	if zone == "" {
		return types.StringNull()
	}
	return types.StringValue(zone)
}

// Only a positive daemon listing confirms an outage; other read errors remain errors.
func unavailableInstance(ctx context.Context, client multipasscli.Client, name string) *models.Instance {
	instances, err := client.ListInstances(ctx, true)
	if err != nil {
		return nil
	}
	for _, instance := range instances {
		if instance.Name == name && strings.EqualFold(instance.State, "Unavailable") {
			return &instance
		}
	}
	return nil
}
