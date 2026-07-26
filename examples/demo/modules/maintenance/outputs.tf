output "ids" {
  description = "The three windows this module created."
  value = {
    weekly  = uptimekuma_maintenance.weekly.id
    one_off = uptimekuma_maintenance.one_off.id
    manual  = uptimekuma_maintenance.manual.id
  }
}

output "weekly_status" {
  description = "Server-computed status of the recurring window."
  value       = uptimekuma_maintenance.weekly.status
}
