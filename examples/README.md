# Examples Overview

This provider ships with progressively complex Terraform examples. Each scenario highlights different Multipass capabilities so you can mix-and-match patterns in your own projects.

## 1. `basic/`
Minimal single instance showing required provider configuration and a simple alias. Good starting point for smoke tests.

## 2. `dev-lab/`
Multi-instance development lab:
- `multipass_instance` resources for `db`, `api`, and `web` tiers with different sizing.
- Shared defaults via locals, plus dependency ordering using `depends_on`.
- `multipass_alias` shortcuts (e.g., `dev-shell`, `db-shell`).
- Data source usage (`multipass_images`) to pin a specific release.

## 3. `bridged-workstation/`
Focuses on networking and host integration:
- Uses `multipass_networks` data to select a real bridge-able NIC.
- Configures multiple networks per instance (Wi-Fi + Ethernet) and a host mount for source code.
- Demonstrates `multipass_alias` with `working_directory` for ergonomic host commands.

## 4. `cloud-init-lab/`
Showcases cloud-init automation:
- External `cloud-init` YAML file installs packages, users, and services.
- Combines mounts, aliases, and outputs capturing instance IPs.
- Illustrates per-instance metadata adoption (tags via Terraform locals and outputs).

Each subdirectory contains a `main.tf` (and supporting files where needed). Run `terraform init` (or `tofu init`) inside any example directory and the published provider will be installed automatically.

