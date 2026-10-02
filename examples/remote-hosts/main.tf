# Authenticate the local CLI user with both daemons before applying.
# See ../../docs/guides/remote-hosts.md for setup and networking requirements.
terraform {
  required_providers {
    multipass = {
      source = "todoroff/multipass"
    }
  }
}

variable "first_server_address" {
  description = "First daemon endpoint, for example 192.0.2.10:50051."
  type        = string
}

variable "second_server_address" {
  description = "Second daemon endpoint, for example 192.0.2.20:50051."
  type        = string
}

provider "multipass" {
  alias          = "first"
  server_address = var.first_server_address
}

provider "multipass" {
  alias          = "second"
  server_address = var.second_server_address
}

resource "multipass_instance" "first" {
  provider = multipass.first
  name     = "first-dev"
  image    = "lts"
  cpus     = 2
  memory   = "2G"
  disk     = "10G"
}

resource "multipass_instance" "second" {
  provider = multipass.second
  name     = "second-dev"
  image    = "lts"
  cpus     = 2
  memory   = "2G"
  disk     = "10G"
}

data "multipass_instance" "first" {
  provider = multipass.first
  name     = multipass_instance.first.name
}

data "multipass_instance" "second" {
  provider = multipass.second
  name     = multipass_instance.second.name
}

output "instance_addresses" {
  value = {
    first  = data.multipass_instance.first.ipv4
    second = data.multipass_instance.second.ipv4
  }
}
