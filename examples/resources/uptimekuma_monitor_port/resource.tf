resource "uptimekuma_monitor_port" "database" {
  name     = "PostgreSQL"
  hostname = "db.internal"
  port     = 5432
}
