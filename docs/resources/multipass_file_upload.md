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

## Attribute Reference

* `id` – Canonical identifier of the form `<instance>:<destination>`.
* `content_hash` – SHA256 hash of the payload used for drift detection.

## Behavior & Notes

* Updates re-run `multipass transfer` whenever `content_hash` changes, mirroring how Terraform provisioners behave during apply.
* When using `content`, data never touches disk outside of a short-lived temp file that is deleted after the transfer.
* Destroying the resource removes the remote path via `multipass exec <instance> rm -rf -- <destination>`. Use caution when pointing `destination` at directories shared with other resources.

