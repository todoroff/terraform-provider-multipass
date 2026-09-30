<p align="center">
  <img width="600"  alt="terraform-provider-multipass-heading-image" src="https://github.com/user-attachments/assets/61f462de-90b1-4352-80a7-ab6bb140e175" />
</p>

# Terraform Multipass Provider

[Terraform](https://www.terraform.io/) provider for [Canonical Multipass](https://canonical.com/multipass), implemented with the modern Terraform Plugin Framework. It exposes rich instance lifecycle management, alias automation, and data sources for images and networks while favoring performance and clear diagnostics.

## Features

- Provider configuration for CLI discovery, command timeouts, default images, and cached `multipass` metadata.
- `multipass_instance` resource with CPU/memory/disk sizing, multiple networks, host mounts, and inline or file-based cloud-init.
- Configurable resizing: CPU, memory, and disk growth update the existing VM by default; `resize_policy = "replace"` rebuilds it instead.
- `multipass_snapshot` resource for managing named snapshots (create/list/delete/import).
- `multipass_alias` resource for ergonomic host shortcuts into instances.
- `multipass_file_upload` and `multipass_file_download` resources for Terraform-managed file transfers without provisioners.
- Data sources for images, networks, instances, and snapshots to compose dynamic plans.
- Parser-backed CLI abstraction with detailed diagnostics.
- Pulumi examples and bridge compatibility checks for TypeScript and Python.

## Getting Started

1. Install prerequisites:
   - Terraform CLI 1.6 or newer.
   - Go at the version required by `go.mod` (for local development).
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

## Pulumi

Use the provider through Pulumi's [Any Terraform Provider](https://www.pulumi.com/docs/iac/concepts/providers/any-terraform-provider/) bridge. In an existing Pulumi project:

```sh
pulumi package add terraform-provider@1.4.0 registry.terraform.io/todoroff/multipass 2.0.0
pulumi install
```

Start with the [TypeScript or Python examples](examples/pulumi/). The [Pulumi guide](docs/guides/pulumi.md) covers local builds (including Windows), configuration, imports, and lifecycle details such as replacement ordering and the generated `ipv4s` output.

## Provider Configuration

| Attribute        | Type   | Description                                                                 |
| ---------------- | ------ | --------------------------------------------------------------------------- |
| `multipass_path` | String | Optional explicit path to the `multipass` binary. Defaults to PATH lookup. |
| `command_timeout`| Int    | Timeout in seconds for CLI calls (default 600).                             |
| `default_image`  | String | Fallback image alias/name when resources omit `image`.                      |

## Resources

- `multipass_instance`: manages VM lifecycle. Supports optional `networks` and `mounts` nested blocks, cloud-init file references, and auto-recovery semantics.
- `multipass_alias`: creates host aliases executing commands inside instances.
- `multipass_file_upload`: provision-style file or directory uploads backed by `multipass transfer`, an alternative to Terraform provisioners.
- `multipass_file_download`: pull files or directories from Multipass instances back to the host with Terraform-managed lifecycles.

## Instance resize policy

`multipass_instance.resize_policy` defaults to `"in_place"`. A size change stops and resizes a running VM, then restarts it; a stopped VM remains stopped. In-place disk shrink is rejected. Removing a sizing argument keeps its current allocation.

This changes the previous default of replacing the VM for every size change. Set `resize_policy = "replace"` to retain that behavior, including replacement with a smaller disk. Changing only the policy does not restart or replace the instance. Image, name, network, and cloud-init changes still force replacement.

See the [instance resource documentation](docs/resources/multipass_instance.md#resizing) for downtime, failure recovery, and guest filesystem expansion details.

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

Each Terraform directory is self-contained; run `terraform init` (or `tofu init`) inside the target folder and the published provider will be installed automatically. The [Pulumi examples](examples/pulumi/) use `pulumi install`.

## Development

For local hacking you can still build from source via:

```bash
go build ./cmd/terraform-provider-multipass
```

### Tests

Run unit tests and static checks without creating VMs:

```bash
go test ./...
go vet ./...
```

Acceptance tests use real Multipass instances and remove their test resources afterward. They require a running Multipass daemon and Terraform or OpenTofu. Set the provider namespace because the test configurations use `todoroff/multipass` explicitly.

```powershell
$env:TF_ACC = "1"
$env:TF_ACC_PROVIDER_NAMESPACE = "todoroff"
$env:TF_ACC_TERRAFORM_PATH = (Get-Command tofu).Source # or terraform
go test ./internal/multipasscli ./internal/provider -p 1 -run '^TestAcc' -count=1 -v -timeout 30m
```

On Linux/macOS, set the same environment variables before running the Go command. Leave `TF_ACC` unset for unit tests. The suite covers scoped VM deletion, instance replacement, both resize policies and power-state preservation, cloud-init failure and retry, computed uploads, download contents and ownership, aliases, data sources, and snapshots. Run only resize acceptance tests with `go test ./internal/provider -run '^TestAccInstanceResource_resize' -count=1 -v -timeout 30m` after setting the same environment variables.

### CI & Releases

- CI runs on GitHub Actions (`.github/workflows/ci.yml`) and executes `go test ./...` across a small matrix of Go versions and OSes.
- Pulumi compatibility runs separately in `.github/workflows/pulumi.yml`: schema checks, SDK generation, TypeScript checking, and previews with a fake Multipass CLI on Linux and Windows. See [how to run it locally](docs/guides/pulumi.md#compatibility-checks).
- Tagged releases (`X.Y.Z`) trigger GoReleaser (`.goreleaser.yml`) via `.github/workflows/release.yml`, which builds cross-platform artifacts suitable for attaching to GitHub Releases and publishing to the Terraform/OpenTofu registries.

## License

This project is licensed under the [MIT License](LICENSE).
