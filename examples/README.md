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

## 5. `file-provisioner/`
Provisioner-style workflow using the native `multipass_file_upload` and `multipass_file_download` resources:
- Uploads a shell script from disk, an inline `.env` file, and an entire directory tree.
- Downloads those same artifacts back onto the host, showcasing directory recursion and `triggers`.
- Ideal starting point if you're replacing Terraform's `file` provisioner (or `null_resource` download hacks).

Each subdirectory contains a `main.tf` (and supporting files where needed). Run `terraform init` (or `tofu init`) inside any example directory and the published provider will be installed automatically.

