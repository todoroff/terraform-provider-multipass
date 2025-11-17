terraform {
  required_providers {
    multipass = {
      source  = "todoroff/multipass"
      version = ">= 1.0.0"
    }
  }
}

provider "multipass" {
  # Uses the host's default multipass binary. Override `multipass_path`
  # if you keep Multipass elsewhere.
}

variable "instance_name" {
  type        = string
  description = "Name of the Multipass VM used for the file transfer demo."
  default     = "file-demo"
}

variable "app_version" {
  type        = string
  description = "Version string written to the inline .env file."
  default     = "1.0.0"
}

resource "multipass_instance" "demo" {
  name   = var.instance_name
  cpus   = 1
  memory = "1G"
  disk   = "5G"
}

# Upload a single script from disk, analogous to Terraform's file provisioner.
resource "multipass_file_upload" "bootstrap_script" {
  instance    = multipass_instance.demo.name
  destination = "/home/ubuntu/bootstrap.sh"
  source      = "${path.module}/files/bootstrap.sh"
}

# Upload a rendered inline file without touching disk.
resource "multipass_file_upload" "env_file" {
  instance    = multipass_instance.demo.name
  destination = "/home/ubuntu/.demo.env"
  content = <<-EOT
    PORT=8080
    VERSION=${var.app_version}
    DATABASE_URL=sqlite:///home/ubuntu/app/data.db
  EOT
}

# Recursively copy an entire app bundle directory into Multipass.
resource "multipass_file_upload" "app_bundle" {
  instance    = multipass_instance.demo.name
  destination = "/home/ubuntu/app"
  source      = "${path.module}/files/app"
  recursive   = true
}

# Download the bootstrap script back to the host to demonstrate the download resource.
resource "multipass_file_download" "bootstrap_copy" {
  instance    = multipass_instance.demo.name
  source      = "/home/ubuntu/bootstrap.sh"
  destination = "${path.module}/downloads/bootstrap.sh"
  create_parents = true

  triggers = {
    last_updated = multipass_instance.demo.last_updated
  }

  depends_on = [
    multipass_file_upload.bootstrap_script,
  ]
}

# Download the uploaded app directory to the host.
resource "multipass_file_download" "app_bundle_copy" {
  instance    = multipass_instance.demo.name
  source      = "/home/ubuntu/app"
  destination = "${path.module}/downloads/app"
  recursive   = true
  overwrite   = true
  create_parents = true

  depends_on = [
    multipass_file_upload.app_bundle,
  ]
}

output "uploaded_paths" {
  description = "Helpful paths to inspect after apply."
  value = {
    bootstrap = multipass_file_upload.bootstrap_script.destination
    env_file  = multipass_file_upload.env_file.destination
    app_dir   = multipass_file_upload.app_bundle.destination
  }
}

output "downloaded_paths" {
  description = "Local download targets created by the download resource."
  value = {
    bootstrap_copy = multipass_file_download.bootstrap_copy.destination
    app_bundle     = multipass_file_download.app_bundle_copy.destination
  }
}

