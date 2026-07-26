resource "uptimekuma_monitor_http" "web" {
  name = "Web"
  url  = "https://example.com"
}

# Recurring weekly window: Monday, Wednesday and Friday, 02:00 to 04:30.
resource "uptimekuma_maintenance" "weekly_patching" {
  title       = "Weekly patching"
  description = "OS updates and reboots"
  strategy    = "recurring-weekday"
  weekdays    = [1, 3, 5]
  start_time  = "02:00"
  end_time    = "04:30"
  timezone    = "Europe/Lisbon"

  monitor_ids = [uptimekuma_monitor_http.web.id]
}

# One-off window with a fixed start and end.
resource "uptimekuma_maintenance" "migration" {
  title       = "Database migration"
  description = "Moving to the new cluster"
  strategy    = "single"
  start_date  = "2026-09-12 22:00"
  end_date    = "2026-09-13 02:00"

  monitor_ids = [uptimekuma_monitor_http.web.id]
}
