terraform {
  required_version = ">= 1.6.0"

  required_providers {
    multipass = {
      source  = "todoroff/multipass"
      version = ">= 1.2.0"
    }
  }
}

variable "bridge_name" {
  description = "Host network interface to bridge (see `multipass networks`)."
  type        = string
  default     = "Ethernet"
}

variable "workspace_path" {
  description = "Host directory to mount into the instance."
  type        = string
  default     = "/home/USERNAME/projects"
}

data "multipass_networks" "host" {}

locals {
  fallback_bridge = try(data.multipass_networks.host.networks[0].name, var.bridge_name)
  selected_bridge = coalesce(var.bridge_name, local.fallback_bridge)
}

resource "multipass_instance" "workstation" {
  name   = "workstation"
  image  = "lts"
  cpus   = 4
  memory = "8G"
  disk   = "40G"

  networks {
    name = local.selected_bridge
  }

  networks {
    name = "Wi-Fi"
  }

  mounts {
    host_path     = var.workspace_path
    instance_path = "/workspace"
  }

  primary      = true
  auto_recover = true
}

resource "multipass_alias" "workspace_shell" {
  name              = "workspace-shell"
  instance          = multipass_instance.workstation.name
  command           = "tmux new -A -s dev"
  working_directory = "/workspace"
}

output "workstation_networks" {
  description = "Candidate host networks discovered via multipass."
  value       = data.multipass_networks.host.networks
}

