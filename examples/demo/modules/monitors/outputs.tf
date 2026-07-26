output "offline_safe_ids" {
  description = "Monitors that watch this instance, so they work with no internet access."
  value = [
    uptimekuma_monitor_http.self.id,
    uptimekuma_monitor_port.kuma_port.id,
    uptimekuma_monitor_ping.loopback.id,
    uptimekuma_monitor_docker.kuma_container.id,
  ]
}

output "outbound_ids" {
  description = "Monitors that need outbound access."
  value = [
    uptimekuma_monitor_http.external_api.id,
    uptimekuma_monitor_keyword.keyword.id,
    uptimekuma_monitor_json_query.json_query.id,
    uptimekuma_monitor_dns.dns.id,
  ]
}

output "self_id" {
  value       = uptimekuma_monitor_http.self.id
  description = "The monitor watching this instance."
}

output "port_id" {
  value       = uptimekuma_monitor_port.kuma_port.id
  description = "The TCP port monitor."
}

output "external_api_id" {
  value       = uptimekuma_monitor_http.external_api.id
  description = "The external HTTP monitor."
}

output "push_id" {
  value       = uptimekuma_monitor_push.nightly_job.id
  description = "The push monitor."
}

output "push_url" {
  description = "Path the monitored job should call, relative to the base URL."
  value       = uptimekuma_monitor_push.nightly_job.push_url
}

output "group_ids" {
  description = "The two group monitors."
  value = {
    internal = uptimekuma_monitor_group.internal.id
    external = uptimekuma_monitor_group.external.id
  }
}
