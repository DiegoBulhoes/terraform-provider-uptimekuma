resource "uptimekuma_monitor_group" "production" {
  name = "Production"
}

# Membership is declared on the child, not on the group.
resource "uptimekuma_monitor_http" "web" {
  name      = "Web"
  url       = "https://example.com"
  parent_id = uptimekuma_monitor_group.production.id
}
