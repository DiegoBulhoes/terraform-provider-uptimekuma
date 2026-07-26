# Singleton resource: it adopts the instance's current settings and manages only
# the keys listed here. Destroying it does not revert anything.
resource "uptimekuma_settings" "main" {
  settings = jsonencode({
    keepDataPeriodDays = 180
    checkUpdate        = false
    checkBeta          = false
  })
}

# Discover which keys this Uptime Kuma version supports.
data "uptimekuma_settings" "current" {}

output "available_settings" {
  value = keys(jsondecode(data.uptimekuma_settings.current.settings))
}
