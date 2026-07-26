resource "uptimekuma_monitor_dns" "apex" {
  name            = "example.com A record"
  hostname        = "example.com"
  resolver_server = "1.1.1.1"
  resolve_type    = "A"
}
