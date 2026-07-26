data "uptimekuma_settings" "current" {}

output "retention_days" {
  value = jsondecode(data.uptimekuma_settings.current.settings).keepDataPeriodDays
}
