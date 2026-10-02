---
page_title: "Managing remote Multipass hosts"
description: "Configure remote Multipass daemons and use provider aliases to manage multiple hosts."
---

# Managing remote Multipass hosts

Set `server_address` to select the daemon used by a provider configuration. The provider runs the local `multipass` CLI with `MULTIPASS_SERVER_ADDRESS` set in each command's environment. Resources and data sources share that configured client; different provider aliases have separate clients and metadata caches.

## Prepare each host

1. Install Multipass CLI 1.13+ on the machine running Terraform, and a supported Multipass daemon on each VM host.
2. Configure the remote host's Multipass service to listen on an address reachable from the Terraform machine. The daemon accepts `--address <host>:<port>`; the service configuration depends on the host's operating system and installation. Limit network access to the clients that need it.
3. An already authenticated administrator on each host sets a passphrase interactively with `multipass set local.passphrase`.
4. Configure server certificate trust for the CLI version you use, as described below. Do this after setting the passphrase and allowing the daemon to restart.
5. On the Terraform machine, authenticate with each target daemon as the same OS user that will run Terraform. This authorizes the CLI's client certificate for that daemon.

On Linux/macOS:

```sh
MULTIPASS_SERVER_ADDRESS=192.0.2.10:50051 multipass authenticate
MULTIPASS_SERVER_ADDRESS=192.0.2.10:50051 multipass list --format json
```

On Windows PowerShell:

```powershell
$previousAddress = $env:MULTIPASS_SERVER_ADDRESS
try {
  $env:MULTIPASS_SERVER_ADDRESS = "192.0.2.10:50051"
  multipass authenticate
  multipass list --format json
} finally {
  $env:MULTIPASS_SERVER_ADDRESS = $previousAddress
}
```

Replace the example address with your daemon's endpoint. Repeat for each host. Terraform uses the credentials maintained by the local CLI; authentication is a setup step outside the provider. See Canonical's [authentication guide](https://documentation.ubuntu.com/multipass/latest/how-to-guides/customise-multipass/authenticate-users-with-the-multipass-service/).

### TLS trust with Multipass 1.16

The Windows 1.16.1 CLI verifies the server against Multipass's own CA file. A new remote daemon has a different CA, so `multipass authenticate` can fail during the TLS handshake before the passphrase is checked. `server_address` selects the endpoint; it does not install or change trusted certificates.

For the tested versions, the public CA files are:

- Windows CLI: `C:\ProgramData\Multipass\data\multipass_root_cert.pem`.
- Linux snap daemon: `/var/snap/multipass/common/data/multipassd/multipass_root_cert.pem`.

Obtain the remote public CA through an authenticated channel and verify its fingerprint. Back up the client's existing CA file, then add the remote public CA as another PEM certificate in that file while preserving the local CA and the file's permissions. This is an administrator-managed, application-specific trust bundle shared by all provider aliases. Keep private keys on their original machines. Preserve the original file so a temporary test's trust change can be reversed.

The 1.16.1 CLI also uses `localhost` as its TLS server name. The server certificate must match that name even when `server_address` contains a remote IP or tunnel endpoint. A tested arrangement is to bind the remote daemon to `localhost:50051` and forward it over SSH:

```sh
ssh -N -L 127.0.0.1:55051:127.0.0.1:50051 user@remote-host
```

Then use `server_address = "127.0.0.1:55051"`. Both Multipass listeners remain on loopback; the SSH connection reaches the remote host. Verify the SSH host key before connecting. The tunnel transports the connection, while Multipass still requires its own CA trust and client authentication.

In the live test with daemon 1.16.4, setting `local.passphrase` restarted the daemon and rotated its CA. Recheck the public CA after daemon restarts or upgrades if the handshake starts failing. Confirm that `MULTIPASS_SERVER_ADDRESS=... multipass list --format json` succeeds under the Terraform user before applying.

These details are version-specific. See Canonical's [1.16.1 client TLS implementation](https://github.com/canonical/multipass/blob/v1.16.1/src/client/common/client_common.cpp) and [1.16.4 certificate handling](https://github.com/canonical/multipass/blob/v1.16.4/src/cert/ssl_cert_provider.cpp). The provider keeps TLS verification enabled.

## Select hosts with provider aliases

```hcl
terraform {
  required_providers {
    multipass = {
      source = "todoroff/multipass"
    }
  }
}

provider "multipass" {
  alias          = "first"
  server_address = "192.0.2.10:50051"
}

provider "multipass" {
  alias          = "second"
  server_address = "192.0.2.20:50051"
}

resource "multipass_instance" "first" {
  provider = multipass.first
  name     = "first-dev"
  image    = "lts"
}

resource "multipass_instance" "second" {
  provider = multipass.second
  name     = "second-dev"
  image    = "lts"
}

data "multipass_networks" "second" {
  provider = multipass.second
}
```

Set the matching `provider` on dependent resources and data sources too. For a reusable configuration with variables and outputs, see [examples/remote-hosts](../../examples/remote-hosts/).

## Address precedence

1. An explicit `server_address` overrides the inherited `MULTIPASS_SERVER_ADDRESS` for that provider's CLI subprocesses.
2. Omitting the argument or setting it to `null` preserves the inherited environment variable.
3. If the environment variable is unset or empty, Multipass uses its platform-specific local endpoint.

The provider leaves Terraform's process environment unchanged. Addresses are passed directly to Multipass, which validates the supported address syntax. Explicit values must be non-empty, contain no whitespace or NUL characters, and be known when the provider is configured. A hostname with a port, such as `host:50051`, selects a TCP endpoint; use the CLI's supported local socket address when you need an explicit local endpoint.

Changing `server_address` redirects refresh, apply, import, and destroy to a different daemon. It does not migrate existing instances. Keep each provider alias bound to the host that owns its resources.

## Networking and path behavior

- The Terraform machine needs network access to the daemon endpoint. Commands that use SSH, including `exec`, `wait_for_cloud_init`, and file transfers, also need access to the guest address and SSH port reported by that daemon. A reachable daemon alone does not make a remote host's NAT-only guests reachable.
- `multipass_path`, `cloud_init_file`, upload sources, and download destinations remain paths on the Terraform machine because the CLI runs there. Inline cloud-init and upload content still use stdin.
- Network names refer to the selected daemon's host. Query `multipass_networks` with the same provider alias to find those names.
- Mounts follow Multipass's own behavior: the CLI validates the source path locally, and the daemon uses the supplied path for the mount. Remote mode does not copy or export a local directory to the daemon host. Prefer file uploads when the machines do not share the required filesystem layout.
- Multipass command aliases and `primary = true` remain local CLI settings shared by the OS user. Provider aliases do not isolate those settings. Use unique command-alias names across hosts, and set `MULTIPASS_SERVER_ADDRESS` to the correct daemon when invoking a saved command alias outside Terraform.
