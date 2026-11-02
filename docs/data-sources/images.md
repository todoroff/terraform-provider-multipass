---
page_title: "multipass_images Data Source - multipass"
description: "List and filter launchable Multipass images and blueprints."
---

# multipass_images (Data Source)

Returns the list of images and blueprints reported by `multipass find`.

Multipass 1.17 adds built-in Debian and Fedora images and removes blueprints. Blueprint filtering remains supported for older versions; `kind = "blueprint"` returns an empty list on 1.17. See the [multi-distro example](../../examples/multi-distro/main.tf).

## Example Usage

```hcl
data "multipass_images" "lts" {
  alias = "lts"
}

output "lts_image_name" {
  value = data.multipass_images.lts.images[0].name
}
```

## Argument Reference

| Name    | Type   | Description |
| ------- | ------ | ----------- |
| `name`  | String | Exact image name to match (e.g., `24.04`). |
| `alias` | String | Match images containing the alias (case-insensitive). |
| `kind`  | String | Filter by `image` or `blueprint`. |
| `query` | String | Case-insensitive substring applied to names and descriptions. |

All arguments are optional and can be combined.

## Attributes Reference

`images` is a list of objects with the following attributes:

| Attribute     | Description |
| ------------- | ----------- |
| `name`        | Canonical image name. |
| `aliases`     | List of alias strings. |
| `os`          | Operating system label. |
| `release`     | Human-friendly release description. |
| `remote`      | Remote channel (empty for default). |
| `version`     | Image version tag. |
| `description` | Same as release for images; blueprint descriptions otherwise. |
| `kind`        | `image` or `blueprint`. |

