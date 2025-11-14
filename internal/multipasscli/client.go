package multipasscli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/todoroff/terraform-provider-multipass/internal/models"
)

// Client exposes typed helpers for interacting with the Multipass CLI.
type Client interface {
	Version(ctx context.Context) (string, error)
	ListInstances(ctx context.Context, refresh bool) ([]models.Instance, error)
	GetInstance(ctx context.Context, name string) (*models.Instance, error)
	LaunchInstance(ctx context.Context, opts models.LaunchOptions) error
	StartInstance(ctx context.Context, name string) error
	StopInstance(ctx context.Context, name string, force bool) error
	SuspendInstance(ctx context.Context, name string) error
	RestartInstance(ctx context.Context, name string) error
	DeleteInstance(ctx context.Context, name string, purge bool) error
	RecoverInstance(ctx context.Context, name string) error
	SetPrimary(ctx context.Context, name string) error
	ListImages(ctx context.Context, refresh bool) ([]models.Image, error)
	ListNetworks(ctx context.Context, refresh bool) ([]models.Network, error)
	ListAliases(ctx context.Context, refresh bool) ([]models.Alias, error)
	CreateAlias(ctx context.Context, alias models.Alias) error
	DeleteAlias(ctx context.Context, name string) error
	ListSnapshots(ctx context.Context, instance string) ([]models.Snapshot, error)
	CreateSnapshot(ctx context.Context, instance, name, comment string) (string, error)
	DeleteSnapshot(ctx context.Context, instance, name string, purge bool) error
}

// Config controls the multipass CLI client instantiation.
type Config struct {
	BinaryPath string
	Timeout    int // Seconds
}

type client struct {
	binaryPath string
	timeout    time.Duration

	mu sync.Mutex

	instanceCache *cacheEntry[[]models.Instance]
	imageCache    *cacheEntry[[]models.Image]
	networkCache  *cacheEntry[[]models.Network]
	aliasCache    *cacheEntry[[]models.Alias]
}

const (
	defaultTimeout  = 2 * time.Minute
	cacheTTL        = 3 * time.Second
	jsonFormatFlag  = "--format"
	jsonFormatValue = "json"
)

// NewClient validates the supplied configuration and returns an initialized Client.
func NewClient(ctx context.Context, cfg Config) (Client, error) {
	binary := cfg.BinaryPath
	if binary == "" {
		binary = "multipass"
	}

	if !strings.Contains(binary, "/") && !strings.Contains(binary, "\\") {
		// Look up in PATH to produce early errors.
		if _, err := exec.LookPath(binary); err != nil {
			return nil, fmt.Errorf("unable to find multipass binary %q in PATH: %w", binary, err)
		}
	}

	timeout := defaultTimeout
	if cfg.Timeout > 0 {
		timeout = time.Duration(cfg.Timeout) * time.Second
	}

	return &client{
		binaryPath: binary,
		timeout:    timeout,
	}, nil
}

func (c *client) Version(ctx context.Context) (string, error) {
	var payload versionResponse
	if err := c.runJSON(ctx, &payload, "version"); err != nil {
		return "", err
	}
	return payload.Multipass, nil
}

func (c *client) ListInstances(ctx context.Context, refresh bool) ([]models.Instance, error) {
	c.mu.Lock()
	if !refresh && c.instanceCache.valid(time.Now()) {
		defer c.mu.Unlock()
		return cloneInstances(c.instanceCache.value), nil
	}
	c.mu.Unlock()

	var payload listResponse
	if err := c.runJSON(ctx, &payload, "list"); err != nil {
		return nil, err
	}

	instances := payload.toModel()

	c.mu.Lock()
	c.instanceCache = newCacheEntry(instances, cacheTTL)
	c.mu.Unlock()

	return cloneInstances(instances), nil
}

func (c *client) GetInstance(ctx context.Context, name string) (*models.Instance, error) {
	var payload infoResponse
	if err := c.runJSON(ctx, &payload, "info", name); err != nil {
		if errorsIsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}

	inst, err := payload.toModel(name)
	if err != nil {
		return nil, err
	}

	return inst, nil
}

func (c *client) LaunchInstance(ctx context.Context, opts models.LaunchOptions) error {
	args := []string{"launch"}
	if opts.Name != "" {
		args = append(args, "--name", opts.Name)
	}
	if opts.Image != "" {
		args = append(args, opts.Image)
	}
	if opts.CPUs > 0 {
		args = append(args, "--cpus", fmt.Sprintf("%d", opts.CPUs))
	}
	if opts.Memory != "" {
		args = append(args, "--memory", opts.Memory)
	}
	if opts.Disk != "" {
		args = append(args, "--disk", opts.Disk)
	}
	if opts.CloudInitFile != "" {
		args = append(args, "--cloud-init", opts.CloudInitFile)
	}
	for _, net := range opts.Networks {
		if net.Name == "" {
			continue
		}
		value := net.Name
		var extras []string
		if net.Mode != "" {
			extras = append(extras, fmt.Sprintf("mode=%s", net.Mode))
		}
		if net.Mac != "" {
			extras = append(extras, fmt.Sprintf("mac=%s", net.Mac))
		}
		if len(extras) > 0 {
			value = strings.Join(append([]string{"name=" + net.Name}, extras...), ",")
		}
		args = append(args, "--network", value)
	}
	for _, mount := range opts.Mounts {
		if mount.HostPath == "" || mount.InstancePath == "" {
			continue
		}
		spec := fmt.Sprintf("%s:%s", mount.HostPath, mount.InstancePath)
		if mount.ReadOnly {
			spec = spec + ":ro"
		}
		args = append(args, "--mount", spec)
	}

	if _, err := c.run(ctx, args...); err != nil {
		return err
	}

	c.invalidateInstances()
	return nil
}

func (c *client) StartInstance(ctx context.Context, name string) error {
	return c.runSimple(ctx, "start", name)
}

func (c *client) StopInstance(ctx context.Context, name string, force bool) error {
	args := []string{"stop"}
	if force {
		args = append(args, "--cancel")
	}
	args = append(args, name)
	if _, err := c.run(ctx, args...); err != nil {
		return err
	}
	return nil
}

func (c *client) SuspendInstance(ctx context.Context, name string) error {
	return c.runSimple(ctx, "suspend", name)
}

func (c *client) RestartInstance(ctx context.Context, name string) error {
	return c.runSimple(ctx, "restart", name)
}

func (c *client) DeleteInstance(ctx context.Context, name string, purge bool) error {
	if err := c.runSimple(ctx, "delete", name); err != nil {
		return err
	}
	if purge {
		if err := c.runSimple(ctx, "purge"); err != nil {
			return err
		}
	}
	c.invalidateInstances()
	return nil
}

func (c *client) RecoverInstance(ctx context.Context, name string) error {
	return c.runSimple(ctx, "recover", name)
}

func (c *client) SetPrimary(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("name is required to set primary")
	}
	arg := fmt.Sprintf("client.primary-name=%s", name)
	return c.runSimple(ctx, "set", arg)
}

func (c *client) ListImages(ctx context.Context, refresh bool) ([]models.Image, error) {
	c.mu.Lock()
	if !refresh && c.imageCache.valid(time.Now()) {
		defer c.mu.Unlock()
		return cloneImages(c.imageCache.value), nil
	}
	c.mu.Unlock()

	var payload findResponse
	if err := c.runJSON(ctx, &payload, "find"); err != nil {
		return nil, err
	}

	images := payload.toModel()

	c.mu.Lock()
	c.imageCache = newCacheEntry(images, cacheTTL)
	c.mu.Unlock()

	return cloneImages(images), nil
}

func (c *client) ListNetworks(ctx context.Context, refresh bool) ([]models.Network, error) {
	c.mu.Lock()
	if !refresh && c.networkCache.valid(time.Now()) {
		defer c.mu.Unlock()
		return cloneNetworks(c.networkCache.value), nil
	}
	c.mu.Unlock()

	var payload networksResponse
	if err := c.runJSON(ctx, &payload, "networks"); err != nil {
		return nil, err
	}

	networks := payload.toModel()

	c.mu.Lock()
	c.networkCache = newCacheEntry(networks, cacheTTL)
	c.mu.Unlock()

	return cloneNetworks(networks), nil
}

func (c *client) ListAliases(ctx context.Context, refresh bool) ([]models.Alias, error) {
	c.mu.Lock()
	if !refresh && c.aliasCache.valid(time.Now()) {
		defer c.mu.Unlock()
		return cloneAliases(c.aliasCache.value), nil
	}
	c.mu.Unlock()

	var payload aliasesResponse
	if err := c.runJSON(ctx, &payload, "aliases"); err != nil {
		return nil, err
	}

	aliases := payload.toModel()

	c.mu.Lock()
	c.aliasCache = newCacheEntry(aliases, cacheTTL)
	c.mu.Unlock()

	return cloneAliases(aliases), nil
}

func (c *client) CreateAlias(ctx context.Context, alias models.Alias) error {
	if alias.Name == "" || alias.Instance == "" || alias.Command == "" {
		return fmt.Errorf("alias requires name, instance, and command")
	}
	args := []string{"alias"}
	if alias.WorkingDirectory != "" {
		args = append(args, "--working-directory", alias.WorkingDirectory)
	}
	args = append(args, fmt.Sprintf("%s:%s", alias.Instance, alias.Command))
	args = append(args, alias.Name)

	if _, err := c.run(ctx, args...); err != nil {
		return err
	}
	c.invalidateAliases()
	return nil
}

func (c *client) DeleteAlias(ctx context.Context, name string) error {
	if name == "" {
		return fmt.Errorf("alias name is required")
	}
	if _, err := c.run(ctx, "unalias", name); err != nil {
		return err
	}
	c.invalidateAliases()
	return nil
}

func (c *client) ListSnapshots(ctx context.Context, instance string) ([]models.Snapshot, error) {
	var payload snapshotListResponse
	if err := c.runJSON(ctx, &payload, "list", "--snapshots"); err != nil {
		return nil, err
	}
	return payload.toModel(instance), nil
}

func (c *client) CreateSnapshot(ctx context.Context, instance, name, comment string) (string, error) {
	if instance == "" {
		return "", fmt.Errorf("instance name is required for snapshots")
	}
	args := []string{"snapshot"}
	if name != "" {
		args = append(args, "--name", name)
	}
	if comment != "" {
		args = append(args, "--comment", comment)
	}
	args = append(args, instance)

	out, err := c.run(ctx, args...)
	if err != nil {
		return "", err
	}

	// Output format: "Snapshot taken: instance.snapshotName"
	line := strings.TrimSpace(string(out))
	if line == "" {
		// Fall back to the requested name.
		return name, nil
	}
	parts := strings.Split(line, ":")
	last := strings.TrimSpace(parts[len(parts)-1])
	if last == "" {
		return name, nil
	}
	// last is "instance.snapshot"
	if dot := strings.Index(last, "."); dot != -1 && dot+1 < len(last) {
		return last[dot+1:], nil
	}
	return name, nil
}

func (c *client) DeleteSnapshot(ctx context.Context, instance, name string, purge bool) error {
	if instance == "" || name == "" {
		return fmt.Errorf("instance and snapshot name are required")
	}
	target := fmt.Sprintf("%s.%s", instance, name)
	args := []string{"delete"}
	if purge {
		args = append(args, "--purge")
	}
	args = append(args, target)
	if _, err := c.run(ctx, args...); err != nil {
		return err
	}
	return nil
}

func (c *client) runSimple(ctx context.Context, args ...string) error {
	_, err := c.run(ctx, args...)
	return err
}

func (c *client) runJSON(ctx context.Context, dest any, args ...string) error {
	args = append(args, jsonFormatFlag, jsonFormatValue)
	out, err := c.run(ctx, args...)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(out, dest); err != nil {
		return fmt.Errorf("unable to parse multipass JSON output for %q: %w", strings.Join(args, " "), err)
	}
	return nil
}

func (c *client) run(ctx context.Context, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.binaryPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		return stdout.Bytes(), nil
	}

	if ctx.Err() == context.DeadlineExceeded {
		return nil, fmt.Errorf("multipass command timed out: %s", strings.Join(args, " "))
	}

	stdoutStr := strings.TrimSpace(stdout.String())
	stderrStr := strings.TrimSpace(stderr.String())

	if strings.Contains(stderrStr, "does not exist") || strings.Contains(stderrStr, "not found") {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, stderrStr)
	}

	return nil, &CLIError{
		Command: strings.Join(args, " "),
		Stdout:  stdoutStr,
		Stderr:  stderrStr,
		Err:     err,
	}
}

func (c *client) invalidateInstances() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.instanceCache = nil
}

func (c *client) invalidateAliases() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.aliasCache = nil
}

func cloneInstances(in []models.Instance) []models.Instance {
	out := make([]models.Instance, len(in))
	copy(out, in)
	return out
}

func cloneImages(in []models.Image) []models.Image {
	out := make([]models.Image, len(in))
	copy(out, in)
	return out
}

func cloneNetworks(in []models.Network) []models.Network {
	out := make([]models.Network, len(in))
	copy(out, in)
	return out
}

func cloneAliases(in []models.Alias) []models.Alias {
	out := make([]models.Alias, len(in))
	copy(out, in)
	return out
}

func errorsIsNotFound(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, ErrNotFound)
}
