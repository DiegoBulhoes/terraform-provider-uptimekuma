output "webhook_id" {
  description = "ID of the webhook channel, which is the default for new monitors."
  value       = uptimekuma_notification.webhook.id
}

output "smtp_id" {
  value       = uptimekuma_notification.smtp.id
  description = "ID of the email channel."
}

output "slack_id" {
  value       = uptimekuma_notification.slack.id
  description = "ID of the Slack channel."
}

output "telegram_id" {
  value       = uptimekuma_notification.telegram.id
  description = "ID of the Telegram channel."
}

output "discord_id" {
  value       = uptimekuma_notification.discord.id
  description = "ID of the Discord channel."
}

output "pagerduty_id" {
  value       = uptimekuma_notification.pagerduty.id
  description = "ID of the PagerDuty channel."
}

output "all_ids" {
  description = "Every channel this module created."
  value = [
    uptimekuma_notification.webhook.id,
    uptimekuma_notification.smtp.id,
    uptimekuma_notification.slack.id,
    uptimekuma_notification.telegram.id,
    uptimekuma_notification.discord.id,
    uptimekuma_notification.pagerduty.id,
  ]
}
