# Multipass Terraform Provider

The Multipass provider lets you manage Canonical Multipass instances, aliases, and supporting metadata using Terraform. It uses the Multipass CLI under the hood and requires the CLI to be available on the host running Terraform.

## Configuration

Pulumi users can use the [Terraform bridge guide](guides/pulumi.md) and [TypeScript/Python examples](../examples/pulumi/).

```hcl
provider "multipass" {
  multipass_path  = "/usr/bin/multipass" # optional
  command_timeout = 180                  # optional, seconds
  default_image   = "lts"                # optional
}
```

## Resources

- [`multipass_instance`](resources/instance.md) – manage VM lifecycle, networks, mounts, and metadata.
- [`multipass_alias`](resources/alias.md) – expose commands from instances as host aliases.
- [`multipass_snapshot`](resources/snapshot.md) – manage named snapshots for stopped instances.
- [`multipass_file_upload`](resources/file_upload.md) – upload files, directories, or inline content.
- [`multipass_file_download`](resources/file_download.md) – download files or directories to the host.

## Data Sources

- [`multipass_images`](data-sources/images.md) – enumerate launchable images/blueprints.
- [`multipass_networks`](data-sources/networks.md) – list host bridge targets.
- [`multipass_instance`](data-sources/instance.md) – inspect existing Multipass instances.
- [`multipass_snapshots`](data-sources/snapshots.md) – list snapshots for an instance.

## Examples

Ready-made Terraform configurations live under `examples/`. Start with `basic/`, then explore:

- `dev-lab/` – multi-tier environment (db/api/web) with aliases and outputs.
- `bridged-workstation/` – bridged networking + host mounts.
- `cloud-init-lab/` – provisioning via cloud-init YAML with multiple instances.

Each scenario contains inline comments explaining how to run it; simply run `terraform init` within the example directory to pull the published provider.

Refer to the README for development instructions and examples.
