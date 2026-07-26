output "proxy_id" {
  description = "ID of the egress proxy."
  value       = uptimekuma_proxy.egress.id
}

output "docker_host_id" {
  description = "ID of the Docker host, for docker monitors."
  value       = uptimekuma_docker_host.local.id
}

output "remote_browser_id" {
  description = "ID of the remote browser."
  value       = uptimekuma_remote_browser.chrome.id
}

output "api_key" {
  description = "The generated API key. Only ever returned at creation time."
  value       = uptimekuma_api_key.prometheus.key
  sensitive   = true
}
