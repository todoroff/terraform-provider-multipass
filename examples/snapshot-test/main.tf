terraform {
  required_version = ">= 1.6.0"

  required_providers {
    multipass = {
      source  = "todoroff/multipass"
      version = ">= 1.2.0"
    }
  }
}

provider "multipass" {
  # rely on defaults; ensure multipass is on PATH when running this example
}

resource "multipass_instance" "test" {
  name   = "snapshot-test"
  image  = "lts"
  cpus   = 1
  memory = "1G"
  disk   = "5G"
}

# Note: Multipass requires the instance to be stopped before taking a snapshot.
# This config is intended primarily for schema validation; a real apply should
# stop the instance before creating the snapshot.
resource "multipass_snapshot" "test_snap" {
  instance = multipass_instance.test.name
  name     = "tf-snap"
  comment  = "Terraform snapshot test"
}

data "multipass_snapshots" "test" {
  instance = multipass_instance.test.name
}

output "snapshot_names" {
  value = [for s in data.multipass_snapshots.test.snapshots : s.name]
}


