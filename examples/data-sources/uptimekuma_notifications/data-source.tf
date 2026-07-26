# Attach notification channels created outside Terraform to a managed monitor.
data "uptimekuma_notifications" "all" {}

resource "uptimekuma_monitor_http" "web" {
  name = "Web"
  url  = "https://example.com"

  notification_ids = [
    for notification in data.uptimekuma_notifications.all.notifications :
    notification.id if notification.is_default
  ]
}
