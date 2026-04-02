# Terraform Multipass Provider

Provider source: `todoroff/multipass`
Requires: Multipass CLI >= 1.13 installed on the Terraform host.
This provider shells out to the `multipass` CLI — there is no REST API.

## Quick Start

```hcl
terraform {
  required_providers {
    multipass = {
      source = "todoroff/multipass"
    }
  }
}

provider "multipass" {}

resource "multipass_instance" "dev" {
  name   = "dev-box"
  image  = "lts"
  cpus   = 2
  memory = "4G"
  disk   = "15G"
}
```

## Provider Configuration

| Argument          | Default       | Description                                          |
|-------------------|---------------|------------------------------------------------------|
| `multipass_path`  | `"multipass"` | Path to the `multipass` binary.                      |
| `command_timeout` | `600`         | CLI command timeout in seconds. Must be > 0.         |
| `default_image`   | `"lts"`       | Fallback image when instance omits `image`.          |

## Resources

### multipass_instance

Manages VM lifecycle. Full schema: [docs/resources/multipass_instance.md](docs/resources/multipass_instance.md)

**Arguments:** `name` (required), `image`, `cpus`, `memory`, `disk`, `cloud_init_file`, `cloud_init`, `primary`, `auto_recover`, `auto_start_on_recover`, `wait_for_cloud_init`.
**Nested blocks:** `networks` (name, mode, mac), `mounts` (host_path, instance_path, read_only), `timeouts`.
**Computed:** `id`, `ipv4`, `state`, `release`, `image_release`, `snapshot_count`, `last_updated`.

Key behaviors:
- `cpus`, `memory`, `disk`, `image`, `cloud_init`, `cloud_init_file`, `networks` changes **force recreation**.
- `cloud_init` and `cloud_init_file` are **mutually exclusive**.
- `memory` and `disk` accept Multipass size strings: `"512M"`, `"4G"`, `"1T"`.
- `mounts` can be added/removed **in place** without recreation.
- Import by instance name: `terraform import multipass_instance.dev dev-box`

```hcl
resource "multipass_instance" "app" {
  name   = "my-app"
  image  = "24.04"
  cpus   = 4
  memory = "8G"
  disk   = "30G"

  cloud_init_file     = "${path.module}/cloud-init.yaml"
  wait_for_cloud_init = true

  networks {
    name = "Wi-Fi"
  }

  mounts {
    host_path     = "/home/user/src"
    instance_path = "/workspace"
  }

  auto_recover = true

  timeouts {
    create = "20m"
  }
}
```

For dynamic cloud-init, use `cloud_init` with `templatefile()`:

```hcl
resource "multipass_instance" "runner" {
  name       = "ci-runner"
  cloud_init = templatefile("${path.module}/cloud-init.tpl", {
    username = "ci-runner"
  })
}
```

### multipass_alias

Host-side command alias for an instance. Full schema: [docs/resources/multipass_alias.md](docs/resources/multipass_alias.md)

**Arguments:** `name` (required, recreate on change), `instance` (required), `command` (required), `working_directory` (optional, wraps command with `cd`).

```hcl
resource "multipass_alias" "shell" {
  name              = "app-shell"
  instance          = multipass_instance.app.name
  command           = "bash"
  working_directory = "/workspace"
}
```

Import: `terraform import multipass_alias.shell app-shell`

### multipass_snapshot

Named snapshot of a stopped instance. Full schema: [docs/resources/multipass_snapshot.md](docs/resources/multipass_snapshot.md)

**Arguments:** `instance` (required), `name` (optional, auto-generated if omitted), `comment` (optional). Both `name` and `comment` force recreation.
**Computed:** `id` as `<instance>.<snapshot>`.

The target instance **must be stopped** or the snapshot operation fails.

```hcl
resource "multipass_snapshot" "backup" {
  instance = "my-app"
  name     = "pre-upgrade"
  comment  = "Before major upgrade"
}
```

Import: `terraform import multipass_snapshot.backup my-app.pre-upgrade`

### multipass_file_upload

Transfer files or inline content into an instance. Full schema: [docs/resources/multipass_file_upload.md](docs/resources/multipass_file_upload.md)

**Arguments:** `instance` (required), `destination` (required), `source` or `content` (exactly one required), `recursive`, `create_parents`.
**Computed:** `content_hash` (SHA256, drives update detection).

- Changing `instance` or `destination` forces recreation.
- Updates re-transfer when `content_hash` changes.
- Destroy removes the remote path (`rm -rf`).

```hcl
resource "multipass_file_upload" "config" {
  instance    = multipass_instance.app.name
  destination = "/home/ubuntu/app.conf"
  source      = "${path.module}/files/app.conf"
}

resource "multipass_file_upload" "env" {
  instance    = multipass_instance.app.name
  destination = "/home/ubuntu/.env"
  content     = "DB_HOST=${multipass_instance.db.ipv4[0]}"
}
```

Import: `terraform import multipass_file_upload.config my-app:/home/ubuntu/app.conf`

### multipass_file_download

Copy files from an instance to the host. Full schema: [docs/resources/multipass_file_download.md](docs/resources/multipass_file_download.md)

**Arguments:** `instance`, `source`, `destination` (all required, all force recreation), `recursive`, `create_parents`, `overwrite`, `triggers` (map, forces re-download on change).
**Computed:** `content_hash`.

- Destroy removes the local destination.
- **Cannot be imported.**

```hcl
resource "multipass_file_download" "logs" {
  instance    = multipass_instance.app.name
  source      = "/var/log/cloud-init.log"
  destination = "${path.module}/downloads/cloud-init.log"

  triggers = {
    refresh = multipass_instance.app.last_updated
  }
}
```

## Data Sources

### multipass_images

Enumerate launchable images/blueprints. Full schema: [docs/data-sources/multipass_images.md](docs/data-sources/multipass_images.md)

**Filters (all optional, combinable):** `name` (exact), `alias`, `kind` (`"image"` / `"blueprint"`), `query` (substring).
**Returns:** list `images` with `name`, `aliases`, `os`, `release`, `remote`, `version`, `description`, `kind`.

```hcl
data "multipass_images" "lts" {
  alias = "lts"
}
# Use: data.multipass_images.lts.images[0].name
```

### multipass_networks

List host networks for bridged networking. Full schema: [docs/data-sources/multipass_networks.md](docs/data-sources/multipass_networks.md)

**Filter:** `name` (optional, exact).
**Returns:** list `networks` with `name`, `type`, `description`.

```hcl
data "multipass_networks" "all" {}
```

### multipass_instance (data source)

Read-only inspection of an existing instance. Full schema: [docs/data-sources/multipass_instance.md](docs/data-sources/multipass_instance.md)

**Required:** `name`.
**Returns:** `state`, `release`, `image_release`, `ipv4`, `cpu_count`, `memory_total_bytes`, `memory_used_bytes`, `disk_total_bytes`, `disk_used_bytes`, `snapshot_count`, `last_updated`.

```hcl
data "multipass_instance" "vm" {
  name = "my-vm"
}
```

### multipass_snapshots

List snapshots for an instance. Full schema: [docs/data-sources/multipass_snapshots.md](docs/data-sources/multipass_snapshots.md)

**Required:** `instance`. **Optional:** `name` (exact filter).
**Returns:** list `snapshots` with `instance`, `name`, `comment`, `parent`.

```hcl
data "multipass_snapshots" "all" {
  instance = "my-vm"
}
```

## Import Reference

| Resource                 | Import ID format              | Example                                              |
|--------------------------|-------------------------------|------------------------------------------------------|
| `multipass_instance`     | Instance name                 | `terraform import multipass_instance.dev dev-box`    |
| `multipass_alias`        | Alias name                    | `terraform import multipass_alias.shell app-shell`   |
| `multipass_snapshot`     | `<instance>.<snapshot>`       | `terraform import multipass_snapshot.b my-app.snap1` |
| `multipass_file_upload`  | `<instance>:<destination>`    | `terraform import multipass_file_upload.c vm:/path`  |
| `multipass_file_download`| Not importable                | —                                                    |

## Troubleshooting

**"Multipass version X.Y.Z is below the minimum supported version"**
Upgrade Multipass to >= 1.13. The provider requires JSON output support added in that release.

**Instance creation hangs or times out**
Set `timeouts { create = "20m" }` for large images or slow networks. Increase `command_timeout` at the provider level. If using `cloud_init`, the launch itself may be fast but cloud-init runs async — use `wait_for_cloud_init = true` if downstream resources depend on it.

**"cloud_init" vs "cloud_init_file" conflict**
These are mutually exclusive. Use `cloud_init_file` for a path to a YAML file, or `cloud_init` for inline content (e.g. from `file()` or `templatefile()`).

**Networks changes destroy the instance**
The `networks` block uses `RequiresReplace` — any change to the network list forces recreation. Plan network config before first apply.

**Snapshot fails: "instance is not stopped"**
Multipass requires instances to be stopped before snapshotting. Stop the instance first or use `depends_on` for proper sequencing.

**File upload shows no changes but remote file differs**
The provider tracks content via `content_hash` (SHA256 of the local payload). Remote in-place modifications are not detected.

**File download not re-running**
Use the `triggers` argument with a changing value (e.g. `multipass_instance.app.last_updated`) to force re-download.

**Instance "Deleted" unexpectedly**
If soft-deleted outside Terraform, set `auto_recover = true` to automatically recover. Pair with `auto_start_on_recover = true` to also start it.

## Examples

See [`examples/`](examples/) for complete working configurations:
- `basic/` — minimal instance + alias
- `dev-lab/` — multi-tier lab (DB/API/Web) with dependency ordering
- `bridged-workstation/` — bridged networking, host mounts, primary instance
- `cloud-init-lab/` — cloud-init from file and template
- `file-provisioner/` — upload/download workflows with triggers
