resource "uptimekuma_monitor_ping" "gateway" {
  name     = "Gateway"
  hostname = "10.0.0.1"
  interval = 60
}
