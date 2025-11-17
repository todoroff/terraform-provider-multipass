# multipass_file_download (Resource)

Copies files or directories **from** a Multipass instance back to the host using `multipass transfer`, providing a Terraform-native alternative to ad-hoc `null_resource` download provisioners.

## Example Usage

```hcl
resource "multipass_instance" "dev" {
  name = "dev-shell"
}

resource "multipass_file_download" "bootstrap_copy" {
  instance    = multipass_instance.dev.name
  source      = "/home/ubuntu/bootstrap.sh"
  destination = "${path.module}/downloads/bootstrap.sh"
  create_parents = true
}

resource "multipass_file_download" "logs" {
  instance    = multipass_instance.dev.name
  source      = "/var/log/cloud-init"
  destination = "${path.module}/downloads/cloud-init"
  recursive   = true
  overwrite   = true

  # Re-download whenever the VM's last_update changes:
  triggers = {
    instance_last_updated = multipass_instance.dev.last_updated
  }
}
```

## Argument Reference

* `instance` – (Required) Name of the Multipass instance.
* `source` – (Required) Path inside the instance to download.
* `destination` – (Required) Local filesystem path where the payload will be written. Use the full final path (for directories, this is the destination directory root).
* `recursive` – (Optional) Set to `true` when downloading directories. Defaults to `false`.
* `create_parents` – (Optional) Create missing parent directories for `destination`. Defaults to `true`.
* `overwrite` – (Optional) Whether to overwrite existing files/directories. Defaults to `true`.
* `triggers` – (Optional) Map of arbitrary values that, when changed, force the resource to re-download. This mirrors `null_resource.triggers` and is useful to tie downloads to other resource changes.

## Attribute Reference

* `id` – Identifier in the form `<instance>:<source>-><destination>`.
* `content_hash` – SHA256 hash of the downloaded payload, useful for `triggers` or downstream outputs.

## Behavior & Notes

* Destroying the resource removes the local `destination` to keep parity with Terraform's lifecycle expectations.
* Downloads run during `create`/`update`. To rerun without a configuration change, adjust `triggers`, taint the resource, or use `terraform apply -replace=multipass_file_download.example`.

