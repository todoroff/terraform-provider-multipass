---
page_title: "multipass_zones Data Source - multipass"
description: "List Multipass availability zones, availability, and subnets."
---

# multipass_zones (Data Source)

Lists availability zones using `multipass zones`. Both the CLI and the selected daemon must be version 1.17.0 or newer. Unknown or older versions produce an error; ordinary resources without zone configuration still support older Multipass versions.

## Example Usage

```hcl
data "multipass_zones" "all" {}

output "available_zones" {
  value = [for z in data.multipass_zones.all.zones : z.name if z.available]
}
```

## Argument Reference

| Name | Type | Description |
| --- | --- | --- |
| `name` | String | Optional exact name filter. An unmatched name returns an empty list. |

## Attributes Reference

`zones` is a list sorted by name, containing:

| Attribute | Type | Description |
| --- | --- | --- |
| `name` | String | Zone name, such as `zone1`. |
| `available` | Boolean | Whether the zone is currently available. |
| `subnet` | String | Subnet assigned by the daemon. |

This data source does not change availability or create zones. See the [1.17 guide](../guides/multipass-1.17.md) and [example](../../examples/availability-zones/main.tf).
