---
page_title: "Multipass 1.17 compatibility"
description: "Availability-zone placement, new images, and compatibility boundaries."
---

# Multipass 1.17

The provider retains its Multipass CLI >= 1.13 baseline. Availability-zone configuration and discovery require **both** the CLI and selected daemon to be >= 1.17.0. Remote provider aliases use their own daemon version. Missing or unparseable versions prevent zone operations without disabling older functionality.

## Placement and outages

Set `zone = "zone2"` on an instance to choose placement. Omission on creation lets Multipass choose an available zone. Refresh and import record the daemon's placement; older versions return a null zone. Removing `zone` from configuration retains placement. Changing its explicit value requires replacement and deletes the old VM and its data, regardless of `resize_policy`.

Use [`multipass_zones`](../data-sources/zones.md) to inspect available zones and their subnets. The provider does not create zones or toggle them. An `Unavailable` VM remains managed, retains its last known allocations, and is not auto-recovered. Re-enable its zone before updating it. The instance data source returns null guest metrics while the zone is unavailable. Failed operations retain resource state.

See the [availability-zone example](../../examples/availability-zones/main.tf). Zone availability is mutable: avoid filtering resource `for_each` keys by availability in outage experiments, because removing a key plans destruction. Use a fixed set of zone names for those experiments.

## Images and CLI changes

`image = "debian"` and `image = "fedora"` use the existing launch argument. See the [multi-distro example](../../examples/multi-distro/main.tf). Adapt guest paths and provisioning commands to the selected OS.

Multipass 1.17 removes blueprints. The images data source still reads them on older versions; `kind = "blueprint"` returns an empty list on 1.17. JSON property order is irrelevant to the provider. Guest command exit failures, including cloud-init failures, remain errors with their original CLI exit status; a created VM remains in state if cloud-init fails.

Multipass itself supplies the new HCS and Apple Virtualization drivers. This provider does not select or migrate drivers. See the [upstream release notes](https://github.com/canonical/multipass/releases/tag/v1.17.0).

## Readiness and verification

On 1.17 hosts, bootstrap scripts can run `multipass wait-ready` before Terraform. This waits for the daemon, not for guest cloud-init; use `wait_for_cloud_init` for guest provisioning. The provider does not invoke `wait-ready` automatically.

Local unit/protocol tests cover CLI payloads, version gates, replacement decisions, and simulated outages. Live 1.17 and driver-specific verification are separate. To run the opt-in 1.17 suite on a test host:

```powershell
$env:TF_ACC = "1"
$env:TF_ACC_PROVIDER_NAMESPACE = "todoroff"
$env:TF_ACC_TERRAFORM_PATH = (Get-Command tofu).Source
go test ./internal/provider -run '^TestAcc117' -count=1 -v -timeout 60m
```

The suite creates disposable instances, verifies explicit/default placement and import, launches Debian/Fedora, and checks that a cloud-init reboot changes the boot ID. It skips on older versions and does not disable zones. Ensure the selected daemon is a test host with sufficient capacity and at least one available zone.
