# One window per scheduling strategy, so all three code paths are exercised: the
# recurring ones read timeRange and weekdays, `single` reads dateRange, and
# `manual` reads neither.

resource "uptimekuma_maintenance" "weekly" {
  title       = "Weekly patching"
  description = "OS updates and reboots"
  strategy    = "recurring-weekday"
  weekdays    = [1, 3, 5]
  start_time  = "02:00"
  end_time    = "04:30"

  monitor_ids     = var.monitor_ids
  status_page_ids = var.status_page_ids
}

resource "uptimekuma_maintenance" "one_off" {
  title       = "Database migration"
  description = "Moving to the new cluster"
  strategy    = "single"
  start_date  = "2027-01-15 22:00"
  end_date    = "2027-01-16 02:00"

  monitor_ids = [var.external_monitor_id]
}

resource "uptimekuma_maintenance" "manual" {
  title       = "Manual switch"
  description = "Turned on by hand when something is being worked on"
  strategy    = "manual"
  active      = false
}
