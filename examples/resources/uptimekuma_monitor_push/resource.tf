# A push monitor waits to be called instead of probing anything. The cron job or
# worker being watched calls push_url on every successful run.
resource "uptimekuma_monitor_push" "nightly_backup" {
  name     = "Nightly backup"
  interval = 86400
}

output "backup_heartbeat_url" {
  value = "https://kuma.example.com${uptimekuma_monitor_push.nightly_backup.push_url}"
}
