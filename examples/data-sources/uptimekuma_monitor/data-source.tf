# By ID
data "uptimekuma_monitor" "by_id" {
  id = "12"
}

# Or by name, which must match exactly one monitor
data "uptimekuma_monitor" "by_name" {
  name = "API"
}

output "api_url" {
  value = data.uptimekuma_monitor.by_name.url
}
