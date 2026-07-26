terraform {
  required_providers {
    uptimekuma = {
      source = "DiegoBulhoes/uptimekuma"
    }
  }
}

provider "uptimekuma" {
  endpoint = var.endpoint
  username = var.username
  password = var.password
}
