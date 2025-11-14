terraform {
  required_version = ">= 1.6.0"

  required_providers {
    multipass = {
      source  = "todoroff/multipass"
      version = "1.0.0"
    }
  }
}

locals {
  cloud_init_file = "${path.module}/cloud-init.yaml"
}

resource "multipass_instance" "builder" {
  name            = "ci-builder"
  image           = "lts"
  cpus            = 2
  memory          = "4G"
  disk            = "15G"
  cloud_init_file = local.cloud_init_file

  mounts {
    host_path     = "/home/USERNAME/builds"
    instance_path = "/builds"
  }
}

resource "multipass_instance" "runner" {
  name            = "ci-runner"
  image           = "lts"
  cpus            = 2
  memory          = "3G"
  disk            = "12G"
  cloud_init_file = local.cloud_init_file

  depends_on = [multipass_instance.builder]
}

resource "multipass_alias" "runner_logs" {
  name     = "ci-logs"
  instance = multipass_instance.runner.name
  command  = "journalctl -u nginx -f"
}

output "ci_ips" {
  value = {
    builder = multipass_instance.builder.ipv4
    runner  = multipass_instance.runner.ipv4
  }
}

