module "tags" {
  source = "./modules/tags"
}

module "notifications" {
  source = "./modules/notifications"
}

module "infrastructure" {
  source = "./modules/infrastructure"
}

module "settings" {
  source = "./modules/settings"
}

module "monitors" {
  source = "./modules/monitors"

  endpoint       = var.endpoint
  docker_host_id = module.infrastructure.docker_host_id

  tag_ids = {
    environment = module.tags.environment_id
    team        = module.tags.team_id
  }

  notification_ids = {
    webhook   = module.notifications.webhook_id
    smtp      = module.notifications.smtp_id
    slack     = module.notifications.slack_id
    telegram  = module.notifications.telegram_id
    discord   = module.notifications.discord_id
    pagerduty = module.notifications.pagerduty_id
  }
}

module "status_page" {
  source = "./modules/status-page"

  slug = var.status_page_slug

  offline_safe_monitor_ids = module.monitors.offline_safe_ids
  outbound_monitor_ids     = module.monitors.outbound_ids
  push_monitor_id          = module.monitors.push_id
  linkable_monitor_id      = module.monitors.self_id
}

module "maintenance" {
  source = "./modules/maintenance"

  monitor_ids = [
    module.monitors.self_id,
    module.monitors.port_id,
  ]
  external_monitor_id = module.monitors.external_api_id
  status_page_ids     = [module.status_page.page_id]
}
