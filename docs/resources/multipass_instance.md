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
}
```

## Argument Reference

| Name              | Type    | Required | Description |
| ----------------- | ------- | -------- | ----------- |
| `name`            | String  | Yes      | Multipass instance name. |
| `image`           | String  | No       | Image alias/name. Defaults to provider `default_image` or `lts`. |
| `cpus`            | Number  | No       | Virtual CPU count. Forces recreation. |
| `memory`          | String  | No       | Memory size (`1G`, `512M`, etc.). Forces recreation. |
| `disk`            | String  | No       | Disk size (e.g., `15G`). Forces recreation. |
| `cloud_init_file` | String  | No       | Path to cloud-init YAML applied at launch. Forces recreation. |
| `primary`         | Bool    | No       | If true, mark instance as Multipass primary. |
| `auto_recover`    | Bool    | No       | Attempt to `multipass recover` if the instance is soft-deleted outside Terraform. |
| `networks`        | Block   | No       | Optional repeated block configuring host networks. Attributes: `name` (required), `mode`, `mac`. |
| `mounts`          | Block   | No       | Optional repeated block configuring host mounts at launch. Attributes: `host_path`, `instance_path`, `read_only`. |

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


