# Pulumi examples

Use [TypeScript](typescript/) or [Python](python/) to create a VM, host shell alias, and uploaded greeting file through Pulumi's Terraform bridge. Both examples query LTS images and export the VM's name and IPv4 addresses.

Prerequisites: Multipass 1.13+, Pulumi CLI 3.266.0, and Node.js 22+ or Python 3.10+ with pip/venv. `Pulumi.yaml` pins bridge 1.4.0 and provider 2.0.0.

```sh
cd typescript
# Or: cd python
pulumi login --local
pulumi install
pulumi stack init dev
pulumi preview
pulumi up
```

The default VM names are `pulumi-ts-dev` and `pulumi-py-dev`. Override with `pulumi config set instanceName my-box` if that name is already in use. Optional project settings are `multipassPath`, `hostPath`, and `network`. The CLI and paths must exist on the host running Pulumi.

`pulumi install` generates and links the SDK in `sdks/multipass`. It may update the language dependency manifest and lockfile; keep those changes with your project. The TypeScript example can then be checked with `npm run check` (`npm.cmd run check` in Windows PowerShell).

Run `pulumi destroy` to remove the example's VM, alias, and uploaded file. See the [Pulumi guide](../../docs/guides/pulumi.md) for configuration, schema naming, imports, replacement ordering, secrets, timeouts, and testing local provider changes.
