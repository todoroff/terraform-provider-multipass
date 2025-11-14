# Data Source: multipass_snapshots

Returns snapshots for a given Multipass instance.

## Example Usage

```hcl
data "multipass_snapshots" "db" {
  instance = "lab-db"
}

output "db_snapshot_names" {
  value = [for s in data.multipass_snapshots.db.snapshots : s.name]
}
```

Filter by snapshot name:

```hcl
data "multipass_snapshots" "pre_upgrade" {
  instance = "lab-db"
  name     = "pre-upgrade"
}
```

## Argument Reference

| Name       | Type   | Description |
| ---------- | ------ | ----------- |
| `instance` | String | Name of the Multipass instance whose snapshots to list (required). |
| `name`     | String | Optional exact snapshot name filter. |

## Attributes Reference

`snapshots` is a list of objects with:

| Attribute | Description |
| --------- | ----------- |
| `instance`| Instance name. |
| `name`    | Snapshot name. |
| `comment` | Snapshot comment, if any. |
| `parent`  | Parent snapshot, if reported by Multipass. |


