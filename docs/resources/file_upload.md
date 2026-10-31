---
page_title: "multipass_file_upload Resource - multipass"
description: "Upload local files, directories, or inline content to a Multipass instance."
---

# multipass_file_upload (Resource)

Transfers local files, inline content, or entire directories into a Multipass instance—mirroring Terraform's built-in [`file` provisioner](https://developer.hashicorp.com/terraform/language/provisioners)—by shelling out to `multipass transfer`.

## Example Usage

```hcl
resource "multipass_instance" "dev" {
  name = "dev-shell"
}

resource "multipass_file_upload" "bootstrap" {
  instance    = multipass_instance.dev.name
  destination = "/home/ubuntu/bootstrap.sh"
  source      = "${path.module}/files/bootstrap.sh"

  # optional knobs mirroring `multipass transfer`
  recursive      = false
  create_parents = true
}

resource "multipass_file_upload" "cloud_cfg" {
  instance    = multipass_instance.dev.name
  destination = "/home/ubuntu/cloud.cfg"
  content     = file("${path.module}/templates/cloud.cfg.tmpl")
}
```

To upload an entire directory, enable recursion:

```hcl
resource "multipass_file_upload" "app_bundle" {
  instance    = multipass_instance.dev.name
  destination = "/opt/app"
  source      = "${path.module}/dist"
  recursive   = true
}
```

## Argument Reference

* `instance` – (Required) Name of the target Multipass instance.
* `destination` – (Required) Absolute or relative path inside the instance where the payload is placed.
* `source` – (Optional) Local file or directory to upload. Conflicts with `content`.
* `content` – (Optional) Inline data to upload. Conflicts with `source`.
* `recursive` – (Optional) Whether to copy directories recursively. Defaults to `false`. Must be `true` when `source` points to a directory.
* `create_parents` – (Optional) Whether to create parent directories automatically (`multipass transfer --parents`). Defaults to `true`.

Exactly one of `source` or `content` must be provided.

* `timeouts` – (Optional) Per-operation timeouts (`create`, `update`). Accepts duration strings like `"5m"` or `"1h"`. Falls back to the provider `command_timeout` when not set.

## Attribute Reference

* `id` – Canonical identifier of the form `<instance>:<destination>`.
* `content_hash` – SHA256 hash of the payload used for drift detection.
* `resolved_destination` – Exact remote path owned by the resource, including the child filename when uploading into an existing directory. Unset for legacy recursive uploads until a successful update records ownership.
* `directory` – Whether the owned payload is a directory. Unset when legacy ownership is unknown.

## Behavior & Notes

* Updates re-run `multipass transfer` whenever `content_hash` changes, mirroring how Terraform provisioners behave during apply.
* Inline `content` is sent through stdin. When targeting an existing directory, Multipass names this file `stream_output.dat`.
* Destroy removes only `resolved_destination`. File payloads use nonrecursive deletion; directory payloads own their complete directory tree. Deletion failures retain the resource in Terraform state. Legacy recursive uploads with unknown ownership retain their remote files with a warning.
* Updates keep writing to the recorded path, even if the local source filename changes. Changing between a file and a directory requires explicit resource replacement. Existing directory uploads use private guest staging to merge contents into the same owned root.
* Guest staging cleanup gets up to 30 additional seconds after an upload finishes, fails, or is canceled. Cleanup failures report the retained staging path without hiding the original transfer error.

Directory hashes include entry boundaries and types. For unchanged legacy recursive uploads, refresh upgrades the stored hash without transferring files. Changed local payloads still produce an update plan.

### Upgrading existing upload state

Existing configurations and state continue to work without re-import or changing `destination`. Older file uploads resolve the owned child path during refresh.

Older recursive uploads did not record whether `destination` was an existing parent or the newly created payload root. Refresh preserves these resources and warns that ownership is unknown. Destroy removes their Terraform record and retains the remote files with a warning; it never guesses which directory is safe to delete.

The next successful update follows the previous Multipass transfer behavior and records its exact output path. In particular, an existing directory destination receives a child named after the local source directory. If the first upload originally created the destination itself, an update can therefore create a nested directory, just as with the previous provider. Earlier files outside the newly recorded path are retained. Subsequent updates use the recorded path, and destroy removes only that path. Review the plan and inspect retained files before cleaning them up manually.
