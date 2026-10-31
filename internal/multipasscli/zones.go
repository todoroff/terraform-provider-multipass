package multipasscli

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/go-version"
	"github.com/todoroff/terraform-provider-multipass/internal/models"
)

func RequireZones(versions models.VersionInfo) error {
	minimum := version.Must(version.NewVersion("1.17.0"))
	for _, component := range []struct{ name, raw string }{{"client", versions.Client}, {"daemon", versions.Daemon}} {
		v, err := version.NewVersion(component.raw)
		if err != nil || v.LessThan(minimum) {
			return fmt.Errorf("availability zones require Multipass client and daemon >= 1.17.0; %s version is %q (empty means unknown)", component.name, component.raw)
		}
	}
	return nil
}

func (c *client) ListZones(ctx context.Context) ([]models.Zone, error) {
	var payload map[string]models.Zone
	if err := c.runJSON(ctx, &payload, "zones"); err != nil {
		return nil, err
	}
	zones := make([]models.Zone, 0, len(payload))
	for name, zone := range payload {
		zone.Name = name
		zones = append(zones, zone)
	}
	sort.Slice(zones, func(i, j int) bool { return zones[i].Name < zones[j].Name })
	return zones, nil
}
