# Resource: multipass_instance

Manages the lifecycle of a Canonical Multipass instance via the Multipass CLI. The resource launches instances, tracks their status, and optionally designates an instance as primary.

## Example Usage

```hcl
resource "multipass_instance" "dev" {
  name   = "dev-box"
  image  = "lts"
  cpus   = 2
  memory = "4G"
  disk   = "15G"

  cloud_init_file = "${path.module}/cloud-init.yaml"

  networks {
    name = "Wi-Fi"
  }

  mounts {
    host_path     = "/home/USERNAME/projects"
    instance_path = "/workspace"
  }

  primary      = true
  auto_recover = true

  timeouts {
    create = "20m"
  }
}
```

Inline cloud-init can be supplied directly from Terraform expressions:

```hcl
resource "multipass_instance" "inline" {
  name   = "dev-inline"
  image  = "24.04"

  cloud_init = file("${path.module}/cloud-init.yaml")
}
```

For dynamic cloud-init content, render a template using `templatefile`:

```hcl
locals {
  rendered_cloud_init = templatefile("${path.module}/cloud-init.tpl", {
    username = "ci-runner"
    motd     = "Runner ready!"
  })
}

resource "multipass_instance" "templated" {
  name   = "dev-templated"
  image  = "24.04"
  cpus   = 2

  cloud_init = local.rendered_cloud_init
}
```

See `examples/cloud-init-lab` for a full template-driven setup.

## Argument Reference

| Name              | Type    | Required | Description |
| ----------------- | ------- | -------- | ----------- |
| `name`            | String  | Yes      | Multipass instance name. Forces recreation. |
| `image`           | String  | No       | Image alias/name. Defaults to provider `default_image` or `lts`. Forces recreation. |
| `cpus`            | Number  | No       | Positive virtual CPU count. Defaults to 1 at creation; omission retains the current allocation. Changes follow `resize_policy`. |
| `memory`          | String  | No       | Memory allocation (`1G`, `512M`, or bytes). Defaults to `1G` at creation; omission retains the current allocation. Changes follow `resize_policy`. |
| `disk`            | String  | No       | Disk allocation (e.g., `15G` or bytes). Defaults to `5G` at creation; omission retains the current allocation. In-place changes only permit growth. |
| `resize_policy`   | String  | No       | `in_place` (default) resizes the existing instance; `replace` recreates it when CPU, memory, or disk allocations change. |
| `cloud_init_file` | String  | No       | Path to cloud-init YAML applied at launch. Mutually exclusive with `cloud_init`. Forces recreation. |
| `cloud_init`      | String  | No       | Inline cloud-init YAML applied at launch. Mutually exclusive with `cloud_init_file`. Forces recreation. |
| `primary`         | Bool    | No       | If true, mark instance as Multipass primary. |
| `auto_recover`    | Bool    | No       | Attempt to `multipass recover` if the instance is soft-deleted outside Terraform. |
| `auto_start_on_recover` | Bool | No    | If true, automatically start the instance after a successful `auto_recover`. |
| `wait_for_cloud_init` | Bool | No     | Wait for cloud-init to finish after launch before marking the resource as created. Useful when downstream resources depend on packages or configuration applied by cloud-init. |
| `networks`        | Block   | No       | Optional repeated block configuring host networks. Attributes: `name` (required), `mode`, `mac`. |
| `mounts`          | Block   | No       | Optional repeated block configuring host mounts. Attributes: `host_path`, `instance_path`, `read_only`. |
| `timeouts`        | Block   | No       | Per-operation timeouts (`create`, `read`, `update`, `delete`). Accepts duration strings like `"20m"` or `"1h"`. Falls back to the provider `command_timeout` when not set. |

## Behavior

### Resizing

```hcl
resource "multipass_instance" "dev" {
  name          = "dev-box"
  cpus          = 4
  memory        = "8G"
  disk          = "30G"
  resize_policy = "in_place" # Default; use "replace" to rebuild on size changes.
}
```

| Policy | CPU or memory change | Disk growth | Disk shrink |
| --- | --- | --- | --- |
| `in_place` | Resize the existing VM | Expand the existing disk | Reject before stopping or modifying the VM |
| `replace` | Recreate the VM | Recreate the VM | Recreate with the smaller disk |

In-place resizing gracefully stops a running instance, changes its allocations, and restarts it. This causes downtime. An instance that was already stopped remains stopped. Suspended and transitional instances must be started or stopped before resizing. Equivalent sizes, such as `1G` and `1024M`, do not cause a resize or replacement. Changing only `resize_policy` also leaves the VM running or stopped as it was.

**Upgrade behavior:** CPU, memory, and disk changes now default to in-place resizing. Set `resize_policy = "replace"` to retain the previous replacement behavior. Replacement removes the existing VM and its data through Terraform's normal lifecycle; `lifecycle.prevent_destroy` can block it. Name, image, networks, and cloud-init changes still require replacement regardless of the resize policy.

Removing `cpus`, `memory`, or `disk` from an existing resource keeps its allocation. Refresh and import read configured allocations from Multipass, including while stopped. Sizes support bytes and binary units (`K`, `M`, `G`, `T`, including `KB`/`KiB` forms); decimal unit values are floored to whole bytes. Values must be positive and fit in a signed 64-bit byte count. Multipass may impose additional driver or image limits.

**Readback precision:** Multipass currently rounds memory and disk settings to one decimal place, even with `get --raw`. The provider retains a known configured size when it matches that rounded report. Newly discovered sizes, including imports, are recorded as byte estimates derived from the report. External changes within the same rounding interval cannot be detected. Disk-shrink preflight uses the retained allocation or reported estimate; Multipass independently rejects any shrink that this limited readback cannot identify.

The policy must be known during planning when sizing may change. In-place disk shrink fails during apply preflight, before any mutation. To request a smaller replacement disk, set `resize_policy = "replace"` or explicitly request replacement with Terraform's `-replace` option.

The resize uses `timeouts.update`, falling back to `command_timeout`. If a command fails, the provider stops further changes and records successful allocations. It attempts to restart an originally running VM if it is confirmed stopped, allowing up to 60 additional seconds for recovery and allocation readback. Request cancellation is respected. The original failure and any recovery failures are reported; the provider never automatically falls back to replacement or shrinks allocations to roll back. Run a fresh plan before retrying.

Disk allocation growth does not guarantee the guest partition and filesystem have expanded. If the guest still reports the old capacity, expand them inside the VM as described in [Multipass's instance modification guide](https://canonical.com/multipass/docs/latest/how-to-guides/manage-instances/modify-an-instance/).

### Mounts and cloud-init

`mounts.read_only` must be `false` or omitted. The Multipass CLI does not support read-only mounts; setting it to `true` is rejected before launch. A `:ro` suffix is a literal part of a Multipass mount path, not a permissions option.

With `wait_for_cloud_init = true`, a cloud-init failure or timeout fails creation and blocks dependent resources. The created VM remains recorded in Terraform state so it can be destroyed or replaced on retry.

## Attributes Reference

| Name             | Description |
| ---------------- | ----------- |
| `id`             | Instance name. |
| `ipv4`           | List of IPv4 addresses. |
| `state`          | Instance state (`Running`, `Stopped`, etc.). |
| `release`        | OS release running inside the VM. |
| `image_release`  | Image release metadata from Multipass. |
| `snapshot_count` | Number of snapshots recorded. |
| `last_updated`   | RFC3339 timestamp of last refresh. |

## Import

Existing instances can be imported by name:

```bash
terraform import multipass_instance.dev dev-box
```
