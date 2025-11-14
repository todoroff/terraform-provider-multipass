# Resource: multipass_alias

Creates a host-side alias that executes a command inside a Multipass instance.

## Example Usage

```hcl
resource "multipass_alias" "ls_workspace" {
  name     = "ls-workspace"
  instance = multipass_instance.dev.name
  command  = "ls -lah /workspace"
}
```

## Argument Reference

| Name                | Type   | Required | Description |
| ------------------- | ------ | -------- | ----------- |
| `name`              | String | Yes      | Alias name created on the host. Changing re-creates the resource. |
| `instance`          | String | Yes      | Target Multipass instance. |
| `command`           | String | Yes      | Command executed inside the instance. |
| `working_directory` | String | No       | Working directory within the instance. |

## Attributes Reference

| Name | Description |
| ---- | ----------- |
| `id` | Alias name. |

## Import

An existing alias can be imported by name:

```bash
terraform import multipass_alias.ls_workspace ls-workspace
```


