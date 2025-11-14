# Data Source: multipass_instance

Returns information about an existing Multipass instance without managing it.

## Example Usage

```hcl
data "multipass_instance" "primary" {
  name = "primary"
}

output "primary_state" {
  value = data.multipass_instance.primary.state
}
```

## Argument Reference

| Name  | Type   | Description |
| ----- | ------ | ----------- |
| `name`| String | Name of the Multipass instance to inspect (required). |

## Attributes Reference

| Attribute            | Description |
| -------------------- | ----------- |
| `state`              | Instance state (Running, Stopped, Deleted...). |
| `release`            | OS release running inside the VM. |
| `image_release`      | Release reported by the source image. |
| `ipv4`               | List of IPv4 addresses. |
| `cpu_count`          | Number of CPUs. |
| `memory_total_bytes` | Total memory bytes assigned. |
| `memory_used_bytes`  | Current memory usage (bytes). |
| `disk_total_bytes`   | Total disk bytes. |
| `disk_used_bytes`    | Used disk bytes. |
| `snapshot_count`     | Number of snapshots recorded. |
| `last_updated`       | RFC3339 timestamp of the last refresh. |


