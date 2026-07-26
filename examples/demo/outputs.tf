output "server" {
  description = "What the instance reports about itself."
  value = {
    version      = data.uptimekuma_info.server.version
    database     = data.uptimekuma_info.server.database_type
    is_container = data.uptimekuma_info.server.is_container
  }
}

output "counts" {
  description = "How much of the instance this configuration created."
  value = {
    monitors      = length(data.uptimekuma_monitors.all.monitors)
    http_monitors = length(data.uptimekuma_monitors.http_only.monitors)
    tags          = length(data.uptimekuma_tags.all.tags)
    notifications = length(data.uptimekuma_notifications.all.notifications)
    maintenances  = length(data.uptimekuma_maintenances.all.maintenances)
    status_pages  = length(data.uptimekuma_status_pages.all.status_pages)
    proxies       = length(data.uptimekuma_proxies.all.proxies)
    docker_hosts  = length(data.uptimekuma_docker_hosts.all.docker_hosts)
    api_keys      = length(data.uptimekuma_api_keys.all.api_keys)
  }
}

output "status_page" {
  description = "The status page this configuration built."
  value = {
    url    = "${var.endpoint}/status/${module.status_page.slug}"
    id     = module.status_page.page_id
    groups = [for group in data.uptimekuma_status_page.public.groups : "${group.name} (${length(group.monitor_ids)} monitors)"]
  }
}

output "notification_channels" {
  description = "Every notification channel, by type. The credentials in the module are fake."
  value = sort([
    for channel in data.uptimekuma_notifications.all.notifications : "${channel.type}: ${channel.name}"
  ])
}

output "push_url" {
  description = "Full URL a cron job would call to report the nightly job succeeded."
  value       = "${var.endpoint}${module.monitors.push_url}?status=up&msg=OK"
}

output "monitor_lookup" {
  description = "The monitor found by ID, to show the data source agrees with the resource."
  value = {
    name = data.uptimekuma_monitor.self.name
    tags = length(data.uptimekuma_monitor.self.tags)
  }
}

output "managed_settings" {
  description = "Settings this configuration manages."
  value       = module.settings.managed
}

output "maintenance_windows" {
  description = "The maintenance windows, and the status the server computed for the recurring one."
  value = {
    ids           = module.maintenance.ids
    weekly_status = module.maintenance.weekly_status
  }
}

output "api_key" {
  description = "The generated API key. Only ever returned at creation time."
  value       = module.infrastructure.api_key
  sensitive   = true
}
