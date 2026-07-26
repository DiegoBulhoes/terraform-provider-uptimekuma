resource "uptimekuma_monitor_http" "api" {
  name     = "API"
  url      = "https://api.example.com/health"
  interval = 60

  max_retries           = 3
  accepted_status_codes = ["200-299"]
}
