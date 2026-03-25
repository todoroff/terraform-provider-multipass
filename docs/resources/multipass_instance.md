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
| `name`            | String  | Yes      | Multipass instance name. |
| `image`           | String  | No       | Image alias/name. Defaults to provider `default_image` or `lts`. |
| `cpus`            | Number  | No       | Virtual CPU count. Forces recreation. |
| `memory`          | String  | No       | Memory size (`1G`, `512M`, etc.). Forces recreation. |
| `disk`            | String  | No       | Disk size (e.g., `15G`). Forces recreation. |
| `cloud_init_file` | String  | No       | Path to cloud-init YAML applied at launch. Mutually exclusive with `cloud_init`. Forces recreation. |
| `cloud_init`      | String  | No       | Inline cloud-init YAML applied at launch. Mutually exclusive with `cloud_init_file`. Forces recreation. |
| `primary`         | Bool    | No       | If true, mark instance as Multipass primary. |
| `auto_recover`    | Bool    | No       | Attempt to `multipass recover` if the instance is soft-deleted outside Terraform. |
| `auto_start_on_recover` | Bool | No    | If true, automatically start the instance after a successful `auto_recover`. |
| `wait_for_cloud_init` | Bool | No     | Wait for cloud-init to finish after launch before marking the resource as created. Useful when downstream resources depend on packages or configuration applied by cloud-init. |
| `networks`        | Block   | No       | Optional repeated block configuring host networks. Attributes: `name` (required), `mode`, `mac`. |
| `mounts`          | Block   | No       | Optional repeated block configuring host mounts. Attributes: `host_path`, `instance_path`, `read_only`. |
| `timeouts`        | Block   | No       | Per-operation timeouts (`create`, `read`, `update`, `delete`). Accepts duration strings like `"20m"` or `"1h"`. Falls back to the provider `command_timeout` when not set. |

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


