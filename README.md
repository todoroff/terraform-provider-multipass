<p align="center">
  <img width="600"  alt="terraform-provider-multipass-heading-image" src="https://github.com/user-attachments/assets/61f462de-90b1-4352-80a7-ab6bb140e175" />
</p>

# Terraform Multipass Provider

[Terraform](https://www.terraform.io/) provider for [Canonical Multipass](https://canonical.com/multipass), implemented with the modern Terraform Plugin Framework. It exposes rich instance lifecycle management, alias automation, and data sources for images and networks while favoring performance and clear diagnostics.

## Features

- Provider configuration for CLI discovery, command timeouts, default images, and cached `multipass` metadata.
- `multipass_instance` resource with CPU/memory/disk sizing, multiple networks, host mounts, and inline or file-based cloud-init.
- `multipass_snapshot` resource for managing named snapshots (create/list/delete/import).
- `multipass_alias` resource for ergonomic host shortcuts into instances.
- `multipass_file_upload` and `multipass_file_download` resources for Terraform-managed file transfers without provisioners.
- Data sources for images, networks, instances, and snapshots to compose dynamic plans.
- Parser-backed CLI abstraction with detailed diagnostics.

## Getting Started

1. Install prerequisites:
   - Terraform CLI 1.6 or newer.
   - Go 1.22+ (for local development).
   - Multipass 1.13+ installed and accessible on your PATH (`multipass version --format json` should work).

2. Initialize Terraform/OpenTofu in any configuration that declares `todoroff/multipass` as a required provider. The CLI will download it from the Terraform/Registry automatically. A trimmed version of `examples/basic/main.tf`:

```hcl
terraform {
  required_providers {
    multipass = {
      source = "todoroff/multipass"
    }
  }
}

provider "multipass" {
  multipass_path  = "/usr/bin/multipass" # optional
  command_timeout = 180                  # optional, seconds
  default_image   = "lts"                # optional
}

resource "multipass_instance" "dev" {
  name   = "dev-box"
  image  = "lts"
  cpus   = 2
  memory = "4G"
  disk   = "15G"

  networks {
    name = "en0"
    mode = "manual"            #optional
    mac  = "52:54:00:4b:ab:bd" #optional
  }

  mounts {
    host_path     = "/home/shared"
    instance_path = "/srv/hostshared"
    read_only     = false             # optional
  }
}

resource "multipass_alias" "shell" {
  name     = "dev-shell"
  instance = multipass_instance.dev.name
  command  = "bash"
}
```

## Provider Configuration

| Attribute        | Type   | Description                                                                 |
| ---------------- | ------ | --------------------------------------------------------------------------- |
| `multipass_path` | String | Optional explicit path to the `multipass` binary. Defaults to PATH lookup. |
| `command_timeout`| Int    | Timeout in seconds for CLI calls (default 120).                             |
| `default_image`  | String | Fallback image alias/name when resources omit `image`.                      |

## Resources

- `multipass_instance`: manages VM lifecycle. Supports optional `networks` and `mounts` nested blocks, cloud-init file references, and auto-recovery semantics.
- `multipass_alias`: creates host aliases executing commands inside instances.
- `multipass_file_upload`: provision-style file or directory uploads backed by `multipass transfer`, an alternative to Terraform provisioners.
- `multipass_file_download`: pull files or directories from Multipass instances back to the host with Terraform-managed lifecycles.

## Data Sources

- `multipass_images`: enumerates images/blueprints from `multipass find`, with filters for name, alias, kind, and text query.
- `multipass_networks`: lists bridgable host networks.
- `multipass_instance`: inspects an existing instance for read-only data.
- `multipass_snapshots`: returns snapshots for a target instance with optional name filtering.

## Examples

See `examples/README.md` for scenario overviews. Highlights:

- `examples/basic`: minimal single instance + alias.
- `examples/dev-lab`: multi-instance dev tier (db/api/web) with aliases and outputs.
- `examples/bridged-workstation`: demonstrates bridged networking, host mounts, and working-directory aliases.
- `examples/cloud-init-lab`: shows cloud-init provisioning with supporting YAML.

Each directory is self-contained; run `terraform init` (or `tofu init`) inside the target folder and the published provider will be installed automatically.

## Development

For local hacking you can still build from source via:

```bash
go build ./cmd/terraform-provider-multipass
```

### CI & Releases

- CI runs on GitHub Actions (`.github/workflows/ci.yml`) and executes `go test ./...` across a small matrix of Go versions and OSes.
- Tagged releases (`X.Y.Z`) trigger GoReleaser (`.goreleaser.yml`) via `.github/workflows/release.yml`, which builds cross-platform artifacts suitable for attaching to GitHub Releases and publishing to the Terraform/OpenTofu registries.

## License

This project is licensed under the [MIT License](LICENSE).
