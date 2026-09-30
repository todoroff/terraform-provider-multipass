# Using Multipass with Pulumi

Use this provider through Pulumi's [Any Terraform Provider](https://www.pulumi.com/docs/iac/concepts/providers/any-terraform-provider/) bridge. It wraps the published `todoroff/multipass` binary and generates a local SDK for your language. The same Multipass CLI implementation serves Terraform, OpenTofu, and Pulumi.

## Prerequisites

- Multipass 1.13+ and access to its daemon on the machine running Pulumi.
- Pulumi CLI. The examples and compatibility workflow use **3.266.0**, with the **1.4.0** Terraform bridge and **2.0.0** Multipass provider.
- Node.js 22+ for TypeScript, or Python 3.10+ with pip and venv for Python.

Terraform/OpenTofu and Go are not needed to use the published provider. Go at the version required by `go.mod` is needed to build local changes.

## Quick start

Choose the [TypeScript](../../examples/pulumi/typescript/) or [Python](../../examples/pulumi/python/) example:

```sh
cd examples/pulumi/typescript
# Or: cd examples/pulumi/python
pulumi login --local
pulumi install
pulumi stack init dev
pulumi preview
pulumi up
```

`--local` selects local state; use your existing backend instead if preferred. The example creates one VM, a host shell alias, and an uploaded greeting file. It also queries available LTS images. `pulumi destroy` removes these resources.

Both examples pin the package in `Pulumi.yaml`:

```yaml
packages:
  multipass:
    source: terraform-provider
    version: 1.4.0
    parameters:
      - registry.terraform.io/todoroff/multipass
      - 2.0.0
```

`version` selects the Pulumi bridge; the last parameter selects this Terraform provider. The explicit Terraform registry address avoids depending on an OpenTofu registry mirror.

For an existing Pulumi project, add the same package with:

```sh
pulumi package add terraform-provider@1.4.0 registry.terraform.io/todoroff/multipass 2.0.0
pulumi install
```

The generated imports are `@pulumi/multipass` in TypeScript and `pulumi_multipass` in Python. Generate them with Pulumi; they are local SDKs, not packages to fetch directly from npm or PyPI. Commit the package entry and dependency manifest/lockfile changes. These examples ignore `sdks/` and regenerate it with `pulumi install`.

## Configuration

The examples create an explicit `Provider` and accept these project configuration keys:

```sh
pulumi config set instanceName my-dev-box
pulumi config set multipassPath /usr/bin/multipass --plaintext
# Optional host mount and bridged network:
pulumi config set hostPath /home/me/src
pulumi config set network en0
```

On Windows, quote paths containing spaces:

```powershell
pulumi config set multipassPath 'C:\Program Files\Multipass\bin\multipass.exe' --plaintext
pulumi config set hostPath 'C:\Users\me\src'
```

Omit `multipassPath` to use PATH discovery. The provider's configurable fields are:

| Terraform | TypeScript / YAML | Python | Default |
| --- | --- | --- | --- |
| `multipass_path` | `multipassPath` | `multipass_path` | `multipass` on PATH |
| `command_timeout` | `commandTimeout` | `command_timeout` | 600 seconds |
| `default_image` | `defaultImage` | `default_image` | `lts` at instance creation |

If your program uses the default provider instead of an explicit `Provider`, use `multipass:multipassPath`, `multipass:commandTimeout`, and `multipass:defaultImage` as stack configuration keys. The checked-in examples read the project keys shown above for their explicit provider.

The CLI, mounts, cloud-init files, upload sources, and download destinations all belong to the machine executing Pulumi. A CI runner needs access to the intended Multipass daemon and host paths.

## Schema mapping

The bridge converts snake_case property names to camelCase in TypeScript and YAML. Python keeps snake_case. Lists can be pluralized: the Terraform `ipv4` output becomes **`ipv4s` in both SDKs**.

| Terraform resource | Pulumi class | Pulumi type token |
| --- | --- | --- |
| `multipass_instance` | `Instance` | `multipass:index/instance:Instance` |
| `multipass_alias` | `Alias` | `multipass:index/alias:Alias` |
| `multipass_snapshot` | `Snapshot` | `multipass:index/snapshot:Snapshot` |
| `multipass_file_upload` | `FileUpload` | `multipass:index/fileUpload:FileUpload` |
| `multipass_file_download` | `FileDownload` | `multipass:index/fileDownload:FileDownload` |

| Terraform data source | TypeScript | Python |
| --- | --- | --- |
| `multipass_images` | `getImages` / `getImagesOutput` | `get_images` / `get_images_output` |
| `multipass_networks` | `getNetworks` / `getNetworksOutput` | `get_networks` / `get_networks_output` |
| `multipass_instance` | `getInstance` / `getInstanceOutput` | `get_instance` / `get_instance_output` |
| `multipass_snapshots` | `getSnapshots` / `getSnapshotsOutput` | `get_snapshots` / `get_snapshots_output` |

Use the Output variants when passing resource outputs, so dependencies and unknown values flow through preview correctly. Pass your explicit provider to invokes as well as resources.

`networks` and `mounts` are arrays of objects. `timeouts` is a single object:

```typescript
const vm = new multipass.Instance("dev", {
    name: "dev-box",
    networks: [{ name: "en0" }],
    mounts: [{ hostPath: "/home/me/src", instancePath: "/workspace" }],
    timeouts: { create: "20m", update: "15m" },
}, {
    provider,
    deleteBeforeReplace: true,
    customTimeouts: { create: "30m", update: "20m" },
});
```

In Python, use `InstanceNetworkArgs`, `InstanceMountArgs`, and `InstanceTimeoutsArgs`, as demonstrated in the example. Read-only mounts are not supported by the Multipass CLI; omit `readOnly` / `read_only`.

## Lifecycle details

- **Replacement ordering:** use [`deleteBeforeReplace: true`](https://www.pulumi.com/docs/iac/concepts/resources/options/deletebeforereplace/) (`delete_before_replace=True` in Python) for VMs and named snapshots that retain the same physical name during replacement. A second resource with that name cannot coexist with the first. Use it for file downloads that replace into the same local destination as well, so deleting the old resource cannot remove the new download. Replacement deletes the existing resource and may cause downtime.
- **Resize policy:** `resizePolicy: "in_place"` / `resize_policy="in_place"` is the default. The literal is `in_place`, including the underscore. CPU and memory changes and disk growth update the VM; shrinking a disk fails before mutation. `"replace"` requests recreation for allocation changes. See the [instance resizing guide](../resources/multipass_instance.md#resizing).
- **Cloud-init:** `cloudInit` and `cloudInitFile` are mutually exclusive and force replacement when changed. Set `waitForCloudInit` when uploads or other resources depend on provisioning. A reference to `vm.name` then orders them after creation finishes.
- **Timeouts:** provider `commandTimeout` and resource `timeouts` control CLI operations. Pulumi's `customTimeouts` controls the outer resource operation. Allow time for recovery: launch can poll for up to five additional minutes, and resize recovery can use up to 60 additional seconds.
- **Files and secrets:** `source` takes a local path, not a Pulumi `FileAsset`; `content` takes a string or Output. Inline cloud-init and upload content carry secret annotations in the generated schema. Use `config.requireSecret()` / `config.require_secret()` for secret configuration values. Removing a file upload deletes its remote destination; removing a download deletes its owned local path.
- **Snapshots:** the instance must already be stopped. `dependsOn` orders operations but does not stop a VM; there is no desired power-state argument on `Instance`.

## Import existing resources

Use the resource `import` option with the same IDs accepted by Terraform:

| Resource | Import ID |
| --- | --- |
| `Instance` | `dev-box` |
| `Alias` | `dev-shell` |
| `Snapshot` | `dev-box.pre-upgrade` |
| `FileUpload` | `dev-box:/home/ubuntu/app.conf` |
| `FileDownload` | Not supported |

For example, in a program with the generated SDK and an explicit provider:

```typescript
const existing = new multipass.Instance("existing", {
    name: "dev-box",
}, { provider, import: "dev-box", deleteBeforeReplace: true });
```

In Python, use `pulumi.ResourceOptions(provider=provider, import_="dev-box", delete_before_replace=True)`. Review `pulumi preview` before applying. Remove the import option after adoption, and ensure Terraform and Pulumi do not both manage the same VM. Configure imported file uploads with exactly one of `source` or `content`, matching the existing payload.

## Develop against a local binary

From either example directory in this checkout:

```sh
go build -o ./bin/ ../../../cmd/terraform-provider-multipass
pulumi package add terraform-provider@1.4.0 ./bin/terraform-provider-multipass
pulumi install
pulumi preview
```

Go creates the platform-specific executable in `bin/`. Keep the **Pulumi argument extensionless on Windows**, even though the file is `terraform-provider-multipass.exe`. Bridge 1.4.0 derives the package name from this argument; including `.exe` produces the invalid name `multipass.exe`. Do not pass a release filename containing `_vX.Y.Z` either. Windows resolves the extensionless argument to the `.exe` binary.

Local packages report version `0.0.0` and retain the binary path. Keep it available when running Pulumi. Rebuild the binary for implementation changes and rerun `package add` after schema changes. Restore the published package command before sharing a project that should use registry releases.

## Compatibility checks

Install Pulumi, Node.js, Python, and Go, then run from the repository root:

```sh
PULUMI_TEST=1 go test ./tests/pulumi -count=1 -v -timeout 15m
```

PowerShell:

```powershell
$env:PULUMI_TEST = "1"
go test ./tests/pulumi -count=1 -v -timeout 15m
```

The dedicated workflow runs on Linux and Windows, including changes under `examples/pulumi/`. It builds the current checkout, verifies all resource/data-source tokens and important schema mappings, generates both SDKs, type-checks TypeScript, and previews both examples. It uses a temporary local backend and a fake CLI that only answers version and image queries. No Pulumi account or real VM is needed. These checks cover bridging and preview; real Multipass lifecycle behavior is covered separately by the Terraform acceptance suite.
