# Data Source: multipass_images

Returns the list of images and blueprints reported by `multipass find`.

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


