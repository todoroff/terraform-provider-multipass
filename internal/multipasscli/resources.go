package multipasscli

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/todoroff/terraform-provider-multipass/internal/models"
)

type ResourceSetting string

const (
	ResourceCPUs   ResourceSetting = "cpus"
	ResourceMemory ResourceSetting = "memory"
	ResourceDisk   ResourceSetting = "disk"
)

func (c *client) GetInstanceResources(ctx context.Context, name string) (models.InstanceResources, error) {
	var result models.InstanceResources
	if name == "" {
		return result, fmt.Errorf("instance name is required to read allocations")
	}
	for _, setting := range []ResourceSetting{ResourceCPUs, ResourceMemory, ResourceDisk} {
		key := fmt.Sprintf("local.%s.%s", name, setting)
		out, err := c.run(ctx, "get", "--raw", key)
		if err != nil {
			return result, fmt.Errorf("reading %s: %w", key, err)
		}
		value := strings.TrimSpace(ansiRegex.ReplaceAllString(string(out), ""))
		if setting == ResourceCPUs {
			result.CPUs, err = strconv.ParseInt(value, 10, 64)
			if err == nil && result.CPUs <= 0 {
				err = fmt.Errorf("CPU count must be positive")
			}
		} else {
			var size uint64
			size, err = ParseSize(value)
			if setting == ResourceMemory {
				result.MemoryBytes = size
				result.MemoryDisplay = value
			} else {
				result.DiskBytes = size
				result.DiskDisplay = value
			}
		}
		if err != nil {
			return result, fmt.Errorf("invalid allocation from %s: %w", key, err)
		}
	}
	return result, nil
}

func (c *client) SetInstanceResource(ctx context.Context, name string, setting ResourceSetting, value uint64) error {
	if name == "" {
		return fmt.Errorf("instance name is required to change allocations")
	}
	switch setting {
	case ResourceCPUs, ResourceMemory, ResourceDisk:
	default:
		return fmt.Errorf("unsupported resource setting %q", setting)
	}
	if value == 0 || value > math.MaxInt64 {
		return fmt.Errorf("%s must be positive and fit in a signed 64-bit integer", setting)
	}
	if err := c.runSimple(ctx, "set", fmt.Sprintf("local.%s.%s=%d", name, setting, value)); err != nil {
		return err
	}
	c.invalidateInstances()
	return nil
}
