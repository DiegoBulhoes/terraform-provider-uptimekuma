# One resource covers all 98 notification providers Uptime Kuma ships: the
# provider-specific options live in `settings` as JSON, which is exactly how the
# server stores them.
#
# Every credential here is fake on purpose. The channels are created and
# configured correctly; only delivery would fail, which shows up as an error from
# the "Test" button in the UI.

resource "uptimekuma_notification" "webhook" {
  name       = "Webhook with auth header"
  type       = "webhook"
  is_default = true

  settings = jsonencode({
    webhookURL         = "https://example.com/hooks/uptime"
    webhookContentType = "custom"
    webhookCustomBody  = "{\"text\": \"{{ msg }}\"}"
    # Note the JSON inside JSON: the server wants this field as a string that
    # contains JSON, not as an object.
    webhookAdditionalHeaders = "{\"Authorization\": \"Bearer not-a-real-token\"}"
  })
}

resource "uptimekuma_notification" "smtp" {
  name = "Email to ops"
  type = "smtp"

  settings = jsonencode({
    smtpHost     = "smtp.example.com"
    smtpPort     = 587
    smtpSecure   = false
    smtpFrom     = "uptime@example.com"
    smtpTo       = "ops@example.com"
    smtpUsername = "uptime"
    smtpPassword = "not-a-real-password"
  })
}

resource "uptimekuma_notification" "slack" {
  name = "Slack #alerts"
  type = "slack"

  settings = jsonencode({
    slackwebhookURL = "https://hooks.slack.com/services/T00000000/B00000000/not-a-real-token"
    slackchannel    = "#alerts"
    slackusername   = "Uptime Kuma"
    slackiconemo    = ":rotating_light:"
  })
}

resource "uptimekuma_notification" "telegram" {
  name = "Telegram"
  type = "telegram"

  settings = jsonencode({
    telegramBotToken     = "0000000000:not-a-real-bot-token"
    telegramChatID       = "-1000000000000"
    telegramSendSilently = false
  })
}

resource "uptimekuma_notification" "discord" {
  name = "Discord"
  type = "discord"

  settings = jsonencode({
    discordWebhookUrl = "https://discord.com/api/webhooks/000000000000000000/not-a-real-token"
    discordUsername   = "Uptime Kuma"
  })
}

# Mind the capital letters: the type is the provider's own name, and Uptime Kuma
# is not consistent about case. `PagerDuty` works, `pagerduty` does not.
resource "uptimekuma_notification" "pagerduty" {
  name = "PagerDuty"
  type = "PagerDuty"

  settings = jsonencode({
    pagerdutyIntegrationKey = "not-a-real-integration-key"
    pagerdutyPriority       = "warning"
    pagerdutyAutoResolve    = "resolve"
  })
}
