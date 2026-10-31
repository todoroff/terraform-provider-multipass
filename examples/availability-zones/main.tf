terraform {
  required_providers {
    multipass = {
      source = "todoroff/multipass"
    }
  }
}

provider "multipass" {}

# Requires a provider build with zone support and Multipass CLI/daemon >= 1.17.
data "multipass_zones" "all" {}

resource "multipass_instance" "web" {
  # Fixed keys keep temporary outages from removing VMs from configuration.
  for_each = toset(["zone1", "zone2", "zone3"])
  name     = "web-${each.key}"
  image    = "lts"
  zone     = each.key
}

output "placement" {
  value = { for name, vm in multipass_instance.web : name => {
    zone = vm.zone
    ipv4 = vm.ipv4
  } }
}

output "zones" {
  value = data.multipass_zones.all.zones
}
