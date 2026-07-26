# Singleton: it adopts the instance's current settings and manages only the keys
# listed here. Destroying it does not revert them, because Uptime Kuma has no way
# to delete a setting.

resource "uptimekuma_settings" "instance" {
  settings = jsonencode({
    keepDataPeriodDays = 180
    checkUpdate        = false
    checkBeta          = false
  })
}
