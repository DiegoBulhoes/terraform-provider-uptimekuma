# Every monitor
data "uptimekuma_monitors" "all" {}

# Only the HTTP ones
data "uptimekuma_monitors" "http" {
  type = "http"
}

output "http_monitor_names" {
  value = [for monitor in data.uptimekuma_monitors.http.monitors : monitor.name]
}
