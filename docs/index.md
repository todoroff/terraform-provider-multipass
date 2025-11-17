# Multipass Provider

The Multipass provider lets you manage Canonical Multipass instances, aliases, and supporting metadata using Terraform. It shells out to the `multipass` CLI and therefore requires Multipass to be installed and available on the host where Terraform runs.

## Example Usage

```hcl
terraform {
  required_providers {
    multipass = {
      source  = "todoroff/multipass"
      version = ">= 1.0.0"
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
- `command_timeout` – Optional. Timeout for CLI commands, in seconds. Default: `120`.
- `default_image` – Optional. Default image alias/name used when `multipass_instance.image` is omitted.

## Resources

- `multipass_instance` – Manage VM lifecycle, networks, mounts, and metadata.
- `multipass_alias` – Expose commands from instances as host aliases.
- `multipass_snapshot` – Manage named snapshots for stopped instances.
- `multipass_file_upload` – Provision files or directories into instances using `multipass transfer`.
- `multipass_file_download` – Pull files or directories from instances onto the host.

## Data Sources

- `multipass_images` – Enumerate launchable images/blueprints.
- `multipass_networks` – List host bridge targets.
- `multipass_instance` – Inspect existing Multipass instances.


