package models

import "time"

// Instance represents a Multipass instance as understood by Terraform.
type Instance struct {
	Name          string
	State         string
	Release       string
	ImageRelease  string
	ImageHash     string
	IPv4          []string
	CPUCount      int
	MemoryTotal   uint64
	MemoryUsed    uint64
	DiskTotal     uint64
	DiskUsed      uint64
	Load          []float64
	SnapshotCount int
	Mounts        []Mount
	LastUpdated   time.Time
}

// Mount represents a host to instance mount binding.
type Mount struct {
	HostPath     string
	InstancePath string
	ReadOnly     bool
}

// ImageKind identifies whether an entry originates from regular images or blueprints.
type ImageKind string

const (
	ImageKindImage     ImageKind = "image"
	ImageKindBlueprint ImageKind = "blueprint"
)

// Image captures metadata exposed by `multipass find`.
type Image struct {
	Name        string
	Aliases     []string
	OS          string
	Release     string
	Remote      string
	Version     string
	Description string
	Kind        ImageKind
}

// Network represents host network information for bridging.
type Network struct {
	Name        string
	Type        string
	Description string
}

// Alias models a `multipass alias` entry.
type Alias struct {
	Name             string
	Instance         string
	Command          string
	WorkingDirectory string
}

// LaunchOptions controls instance creation parameters.
type LaunchOptions struct {
	Name          string
	Image         string
	CPUs          int
	Memory        string
	Disk          string
	CloudInitFile string
	Networks      []NetworkAttachment
	Mounts        []Mount
	Primary       bool
}

// NetworkAttachment describes a network interface to attach during launch.
type NetworkAttachment struct {
	Name string
	Mode string
	Mac  string
}
