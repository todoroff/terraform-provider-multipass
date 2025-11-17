# File Provisioner-style Example

This scenario mirrors Terraform's classic `file` provisioner by using the native `multipass_file_upload` resource to push scripts, inline configuration, and entire directories into a Multipass VM—and the companion `multipass_file_download` resource to pull artifacts back to the host.

## Contents

- `main.tf` – spins up a small instance, uploads:
  - `files/bootstrap.sh` (single file on disk)
  - An inline `.env` file rendered from variables
  - `files/app/` directory (recursive copy)
  - Then downloads `/home/ubuntu/bootstrap.sh` and `/home/ubuntu/app` back onto `./downloads/`
- `files/bootstrap.sh` – simple shell script invoked manually inside the instance.
- `files/app/config/app.conf` – part of the directory tree copied into `/home/ubuntu/app`.

## Usage

```bash
cd examples/file-provisioner
terraform init
terraform apply
```

After apply, SSH into the instance with `multipass shell file-demo` (or the name you pass via `-var "instance_name=..."`) and inspect the uploaded files:

```bash
ls /home/ubuntu
cat /home/ubuntu/.demo.env
ls /home/ubuntu/app
```

You can also inspect the host-side downloads at `examples/file-provisioner/downloads/`.

Destroy the stack when finished:

```bash
terraform destroy
```

