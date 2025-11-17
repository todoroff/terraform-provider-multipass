terraform {
  required_version = ">= 1.6.0"

  required_providers {
    multipass = {
      source  = "todoroff/multipass"
      version = ">= 1.2.0"
    }
  }
}

locals {
  lab_prefix     = "lab"
  default_image  = "lts"
  instance_size  = { cpus = 2, memory = "2G", disk = "10G" }
  web_size       = { cpus = 1, memory = "1G", disk = "5G" }
  api_size       = { cpus = 2, memory = "2G", disk = "8G" }
  db_size        = { cpus = 2, memory = "4G", disk = "20G" }
}

data "multipass_images" "lts" {
  alias = local.default_image
}

resource "multipass_instance" "db" {
  name   = "${local.lab_prefix}-db"
  image  = data.multipass_images.lts.images[0].name
  cpus   = local.db_size.cpus
  memory = local.db_size.memory
  disk   = local.db_size.disk

  auto_recover = true
}

resource "multipass_instance" "api" {
  name   = "${local.lab_prefix}-api"
  image  = data.multipass_images.lts.images[0].name
  cpus   = local.api_size.cpus
  memory = local.api_size.memory
  disk   = local.api_size.disk

  auto_recover = true

  # Ensure the DB node is created first so provisioning scripts can reference it.
  depends_on = [multipass_instance.db]
}

resource "multipass_instance" "web" {
  name   = "${local.lab_prefix}-web"
  image  = data.multipass_images.lts.images[0].name
  cpus   = local.web_size.cpus
  memory = local.web_size.memory
  disk   = local.web_size.disk

  auto_recover = true

  depends_on = [multipass_instance.api]
}

resource "multipass_alias" "dev_shell" {
  name              = "lab-shell"
  instance          = multipass_instance.api.name
  command           = "bash"
  working_directory = "/workspace"
}

resource "multipass_alias" "db_shell" {
  name     = "lab-db-shell"
  instance = multipass_instance.db.name
  command  = "psql -U postgres"
}

output "lab_ips" {
  description = "IPv4 addresses for each tier."
  value = {
    db  = multipass_instance.db.ipv4
    api = multipass_instance.api.ipv4
    web = multipass_instance.web.ipv4
  }
}

