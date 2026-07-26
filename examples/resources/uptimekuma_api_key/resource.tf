# API keys authenticate the Prometheus /metrics endpoint. They are not
# credentials for the API this provider uses.
resource "uptimekuma_api_key" "prometheus" {
  name    = "Prometheus scraper"
  expires = "2027-01-01 00:00:00"
}

output "prometheus_api_key" {
  value     = uptimekuma_api_key.prometheus.key
  sensitive = true
}
