---
page_title: "multipass_alias Resource - multipass"
description: "Manage a host-side command alias for a Multipass instance."
---

# multipass_alias (Resource)

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
| `working_directory` | String | No | Working directory inside the instance. The command is automatically wrapped with `cd <dir> && exec <command>`. |

## Attributes Reference

| Name | Description |
| ---- | ----------- |
| `id` | Qualified identity, `<context>.<name>`. |
| `context` | Alias context captured at creation. |

## Import

An existing alias can be imported by name:

```bash
terraform import multipass_alias.ls_workspace ls-workspace
```


Creation uses the current Multipass alias context and records it. Refresh and deletion use that recorded context even after switching contexts. Before updating an alias, select its recorded context with `multipass prefer <context>`; the provider refuses a mismatched context before deleting anything. It does not switch the shared CLI context automatically.

Import with `<context>.<name>` when names occur in multiple contexts. Existing unqualified state is resolved by name and recorded instance; ambiguous matches fail instead of selecting an arbitrary alias.
