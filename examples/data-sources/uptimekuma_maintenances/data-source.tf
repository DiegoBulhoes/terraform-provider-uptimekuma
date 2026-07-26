data "uptimekuma_maintenances" "all" {}

output "active_maintenance_titles" {
  value = [
    for maintenance in data.uptimekuma_maintenances.all.maintenances :
    maintenance.title if maintenance.status == "under-maintenance"
  ]
}
