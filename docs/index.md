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
- `server_address` – Optional. Multipass daemon address, such as `host:50051`. Sets `MULTIPASS_SERVER_ADDRESS` for this provider's CLI commands. When omitted or null, inherits the environment variable, or uses the CLI's local default if it is unset or empty. Explicit values must be non-empty, contain no whitespace or NUL characters, and be known before provider configuration.
- `command_timeout` – Optional. Timeout for CLI commands, in seconds. Default: `600`.
- `default_image` – Optional. Default image alias/name used when `multipass_instance.image` is omitted.

## Remote hosts

Use `server_address` and Terraform provider aliases to manage multiple hosts. The CLI must be installed on the Terraform machine, trust each daemon's TLS certificate, and be authenticated with each daemon. See the [remote-host guide](guides/remote-hosts.md) for the Multipass 1.16 trust setup and the [two-host example](../examples/remote-hosts/).

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
