terraform {
  required_providers {
    uptimekuma = {
      source = "DiegoBulhoes/uptimekuma"
    }
  }
}

provider "uptimekuma" {
  endpoint = "https://kuma.example.com"
  username = "admin"
  password = var.uptime_kuma_password
}

# Or configure it entirely from the environment:
#
#   export UPTIME_KUMA_URL=https://kuma.example.com
#   export UPTIME_KUMA_USERNAME=admin
#   export UPTIME_KUMA_PASSWORD=...
#
# provider "uptimekuma" {}

variable "uptime_kuma_password" {
  type      = string
  sensitive = true
}
