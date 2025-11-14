# Data Source: multipass_networks

Lists host networks available for Multipass bridged networking.

## Example Usage

```hcl
data "multipass_networks" "all" {}

output "network_names" {
  value = [for n in data.multipass_networks.all.networks : n.name]
}
```

## Argument Reference

| Name   | Type   | Description |
| ------ | ------ | ----------- |
| `name` | String | Optional exact match filter for a single network. |

## Attributes Reference

`networks` is a list of objects with:

| Attribute    | Description |
| ------------ | ----------- |
| `name`       | Host network display name. |
| `type`       | Network type (e.g., `ethernet`, `wifi`). |
| `description`| Human-readable description from Multipass. |


