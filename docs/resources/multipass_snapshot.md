# Resource: multipass_snapshot

Manages a named snapshot for a Multipass instance.

> **Note:** Multipass can only take snapshots of **stopped** instances. Ensure the target instance is stopped before applying this resource, or the snapshot operation will fail.

## Example Usage

```hcl
resource "multipass_snapshot" "db_snapshot" {
  instance = "lab-db"
  name     = "pre-upgrade"
  comment  = "Snapshot before DB upgrade"
}
```

## Argument Reference

| Name       | Type   | Required | Description |
| ---------- | ------ | -------- | ----------- |
| `instance` | String | Yes      | Name of the Multipass instance to snapshot. The instance must be stopped. |
| `name`     | String | No       | Snapshot name. If omitted, Multipass will auto-generate one (for example, `snapshot1`). Changing forces recreation. |
| `comment`  | String | No       | Optional snapshot comment. Changing forces recreation. |

## Attributes Reference

| Name | Description |
| ---- | ----------- |
| `id` | Canonical identifier in the form `<instance>.<snapshot>`. |

## Import

An existing snapshot can be imported by the `instance.snapshot` identifier:

```bash
terraform import multipass_snapshot.db_snapshot lab-db.pre-upgrade
```


