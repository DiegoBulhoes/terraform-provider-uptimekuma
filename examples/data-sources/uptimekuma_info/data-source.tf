data "uptimekuma_info" "server" {}

# A containerised instance cannot run the sip-options, tailscale-ping or
# system-service monitor types.
output "server_version" {
  value = data.uptimekuma_info.server.version
}

output "is_container" {
  value = data.uptimekuma_info.server.is_container
}
