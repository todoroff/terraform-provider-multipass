terraform {
  required_providers {
    multipass = {
      source  = "todoroff/multipass"
    }
  }
}

provider "multipass" {}

resource "multipass_instance" "static" {
  name   = "static-ip-demo"
  image  = "lts"
  cpus   = 2
  memory = "4G"
  disk   = "15G"

  # Attach to a specific host NIC from `multipass networks`.
  # Replace "en0" with a valid bridgeable interface on your host.
  networks {
    name = "en0"
    mode = "manual"
    mac  = "52:54:00:4b:ab:bd"
  }

  # Configure a static IP inside the guest via cloud-init/Netplan.
  # You must adjust the address and prefix to match your actual LAN/subnet.
  cloud_init = <<-EOT
    #cloud-config
    write_files:
      - path: /etc/netplan/10-custom.yaml
        permissions: "0644"
        content: |
          network:
            version: 2
            ethernets:
              extra0:
                dhcp4: no
                match:
                  macaddress: "52:54:00:4b:ab:bd"
                addresses: ["192.168.64.97/24"]
    runcmd:
      - netplan apply
  EOT
}

output "instance_ips" {
  description = "Static IP configured inside the guest"
  value       = multipass_instance.static.ipv4
}


