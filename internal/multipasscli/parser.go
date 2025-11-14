package multipasscli

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/todoroff/terraform-provider-multipass/internal/models"
)

type versionResponse struct {
	Multipass string `json:"multipass"`
}

type listResponse struct {
	List []listEntry `json:"list"`
}

type listEntry struct {
	Name    string   `json:"name"`
	State   string   `json:"state"`
	Release string   `json:"release"`
	IPv4    []string `json:"ipv4"`
}

func (r listResponse) toModel() []models.Instance {
	now := time.Now()
	out := make([]models.Instance, 0, len(r.List))
	for _, entry := range r.List {
		out = append(out, models.Instance{
			Name:        entry.Name,
			State:       entry.State,
			Release:     entry.Release,
			IPv4:        sanitizeIPs(entry.IPv4),
			LastUpdated: now,
		})
	}
	return out
}

type infoResponse struct {
	Info map[string]infoEntry `json:"info"`
}

type infoEntry struct {
	CPUCount      string                `json:"cpu_count"`
	Disks         map[string]diskEntry  `json:"disks"`
	ImageHash     string                `json:"image_hash"`
	ImageRelease  string                `json:"image_release"`
	IPv4          []string              `json:"ipv4"`
	Load          []float64             `json:"load"`
	Memory        memoryEntry           `json:"memory"`
	Mounts        map[string]mountEntry `json:"mounts"`
	Release       string                `json:"release"`
	SnapshotCount string                `json:"snapshot_count"`
	State         string                `json:"state"`
}

type diskEntry struct {
	Total string `json:"total"`
	Used  string `json:"used"`
}

type memoryEntry struct {
	Total uint64 `json:"total"`
	Used  uint64 `json:"used"`
}

type mountEntry struct {
	SourcePath string `json:"source_path"`
	ReadOnly   bool   `json:"readonly"`
}

func (r infoResponse) toModel(name string) (*models.Instance, error) {
	entry, ok := r.Info[name]
	if !ok {
		return nil, fmt.Errorf("instance %q missing from info payload", name)
	}

	cpuc, err := strconv.Atoi(entry.CPUCount)
	if err != nil {
		cpuc = 0
	}

	snapshots, err := strconv.Atoi(entry.SnapshotCount)
	if err != nil {
		snapshots = 0
	}

	var diskTotal, diskUsed uint64
	for _, disk := range entry.Disks {
		diskTotal, _ = parseUintString(disk.Total)
		diskUsed, _ = parseUintString(disk.Used)
		break
	}

	mounts := make([]models.Mount, 0, len(entry.Mounts))
	for target, m := range entry.Mounts {
		mounts = append(mounts, models.Mount{
			HostPath:     m.SourcePath,
			InstancePath: target,
			ReadOnly:     m.ReadOnly,
		})
	}
	sort.Slice(mounts, func(i, j int) bool {
		return mounts[i].InstancePath < mounts[j].InstancePath
	})

	return &models.Instance{
		Name:          name,
		State:         entry.State,
		Release:       entry.Release,
		ImageRelease:  entry.ImageRelease,
		ImageHash:     entry.ImageHash,
		IPv4:          sanitizeIPs(entry.IPv4),
		CPUCount:      cpuc,
		DiskTotal:     diskTotal,
		DiskUsed:      diskUsed,
		MemoryTotal:   entry.Memory.Total,
		MemoryUsed:    entry.Memory.Used,
		Load:          entry.Load,
		SnapshotCount: snapshots,
		Mounts:        mounts,
		LastUpdated:   time.Now(),
	}, nil
}

type findResponse struct {
	Images     map[string]findEntry `json:"images"`
	Blueprints map[string]findEntry `json:"blueprints (deprecated)"`
}

type findEntry struct {
	Aliases []string `json:"aliases"`
	OS      string   `json:"os"`
	Release string   `json:"release"`
	Remote  string   `json:"remote"`
	Version string   `json:"version"`
}

func (r findResponse) toModel() []models.Image {
	out := make([]models.Image, 0, len(r.Images)+len(r.Blueprints))
	for name, entry := range r.Images {
		out = append(out, models.Image{
			Name:        name,
			Aliases:     entry.Aliases,
			OS:          entry.OS,
			Release:     entry.Release,
			Remote:      entry.Remote,
			Version:     entry.Version,
			Description: entry.Release,
			Kind:        models.ImageKindImage,
		})
	}
	for name, entry := range r.Blueprints {
		out = append(out, models.Image{
			Name:        name,
			Aliases:     entry.Aliases,
			OS:          entry.OS,
			Release:     entry.Release,
			Remote:      entry.Remote,
			Version:     entry.Version,
			Description: entry.Release,
			Kind:        models.ImageKindBlueprint,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

type networksResponse struct {
	List []networkEntry `json:"list"`
}

type networkEntry struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

func (r networksResponse) toModel() []models.Network {
	out := make([]models.Network, 0, len(r.List))
	for _, entry := range r.List {
		out = append(out, models.Network{
			Name:        entry.Name,
			Type:        entry.Type,
			Description: entry.Description,
		})
	}
	return out
}

type aliasesResponse struct {
	Contexts map[string]map[string]aliasEntry `json:"contexts"`
}

type aliasEntry struct {
	Instance         string `json:"instance"`
	Command          string `json:"command"`
	WorkingDirectory string `json:"working-directory"`
}

func (r aliasesResponse) toModel() []models.Alias {
	out := []models.Alias{}
	for _, ctx := range r.Contexts {
		for name, entry := range ctx {
			out = append(out, models.Alias{
				Name:             name,
				Instance:         entry.Instance,
				Command:          entry.Command,
				WorkingDirectory: entry.WorkingDirectory,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func parseUintString(value string) (uint64, error) {
	if value == "" {
		return 0, fmt.Errorf("empty value")
	}
	return strconv.ParseUint(value, 10, 64)
}

func sanitizeIPs(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" || strings.EqualFold(v, "N/A") {
			continue
		}
		out = append(out, v)
	}
	return out
}
