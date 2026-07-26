resource "uptimekuma_monitor_keyword" "status_page" {
  name    = "Status page says OK"
  url     = "https://example.com/status"
  keyword = "All systems operational"
}
