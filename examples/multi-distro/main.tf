terraform {
  required_providers {
    multipass = {
      source = "todoroff/multipass"
    }
  }
}

provider "multipass" {}

# Multipass 1.17 adds these built-in image aliases. No separate OS setting is needed.
resource "multipass_instance" "lab" {
  for_each            = toset(["debian", "fedora"])
  name                = "lab-${each.key}"
  image               = each.key
  memory              = "2G"
  disk                = "10G"
  wait_for_cloud_init = true

  timeouts {
    create = "20m"
  }
}

output "addresses" {
  value = { for distro, vm in multipass_instance.lab : distro => vm.ipv4 }
}
