---
page_title: "Multipass Provider"
description: "Manage Canonical Multipass virtual machines and supporting resources through the Multipass CLI."
---

# Multipass Provider

The Multipass provider lets you manage Canonical Multipass instances, aliases, and supporting metadata using Terraform. It shells out to the `multipass` CLI and therefore requires Multipass to be installed and available on the host where Terraform runs.

## Example Usage

For Pulumi, see the [Pulumi guide](guides/pulumi.md) and [TypeScript/Python examples](../examples/pulumi/).

```hcl
terraform {
  required_providers {
    multipass = {
      source  = "todoroff/multipass"
    }
  }
}

provider "multipass" {
  multipass_path  = "/usr/bin/multipass" # optional
  command_timeout = 180                  # optional, seconds
  default_image   = "lts"                # optional
}
```

## Argument Reference

The following arguments are supported in the `provider "multipass"` block:

- `multipass_path` – Optional. Explicit path to the `multipass` binary. Defaults to resolving `multipass` on `PATH`.
- `command_timeout` – Optional. Timeout for CLI commands, in seconds. Default: `600`.
- `default_image` – Optional. Default image alias/name used when `multipass_instance.image` is omitted.

## Resources

- [`multipass_instance`](resources/instance.md) – Manage VM lifecycle, networks, mounts, and metadata.
- [`multipass_alias`](resources/alias.md) – Expose commands from instances as host aliases.
- [`multipass_snapshot`](resources/snapshot.md) – Manage named snapshots for stopped instances.
- [`multipass_file_upload`](resources/file_upload.md) – Provision files or directories into instances using `multipass transfer`.
- [`multipass_file_download`](resources/file_download.md) – Pull files or directories from instances onto the host.

## Data Sources

- [`multipass_images`](data-sources/images.md) – Enumerate launchable images/blueprints.
- [`multipass_networks`](data-sources/networks.md) – List host bridge targets.
- [`multipass_instance`](data-sources/instance.md) – Inspect existing Multipass instances.
- [`multipass_snapshots`](data-sources/snapshots.md) – List snapshots for an instance.
