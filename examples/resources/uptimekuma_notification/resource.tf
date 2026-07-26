# The provider-specific options go in `settings`, which is how Uptime Kuma
# stores them. Any of its ~100 notification providers works this way.
resource "uptimekuma_notification" "slack" {
  name       = "Slack #alerts"
  type       = "slack"
  is_default = true

  settings = jsonencode({
    slackwebhookURL = var.slack_webhook_url
    slackchannel    = "#alerts"
    slackusername   = "Uptime Kuma"
  })
}

resource "uptimekuma_notification" "webhook" {
  name = "Internal webhook"
  type = "webhook"

  settings = jsonencode({
    webhookURL         = "https://hooks.internal/uptime"
    webhookContentType = "json"
  })
}

resource "uptimekuma_monitor_http" "web" {
  name             = "Web"
  url              = "https://example.com"
  notification_ids = [uptimekuma_notification.slack.id, uptimekuma_notification.webhook.id]
}

variable "slack_webhook_url" {
  type      = string
  sensitive = true
}
