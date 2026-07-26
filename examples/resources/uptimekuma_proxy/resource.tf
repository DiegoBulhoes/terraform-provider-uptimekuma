resource "uptimekuma_proxy" "egress" {
  protocol = "http"
  host     = "proxy.internal"
  port     = 3128
  username = "kuma"
  password = var.proxy_password
}

resource "uptimekuma_monitor_http" "external" {
  name     = "External API"
  url      = "https://api.partner.example.com/health"
  proxy_id = uptimekuma_proxy.egress.id
}

variable "proxy_password" {
  type      = string
  sensitive = true
}
