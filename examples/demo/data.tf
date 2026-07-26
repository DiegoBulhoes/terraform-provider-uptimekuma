data "uptimekuma_info" "server" {}

data "uptimekuma_monitors" "all" {
  depends_on = [module.monitors]
}

data "uptimekuma_monitors" "http_only" {
  type       = "http"
  depends_on = [module.monitors]
}

data "uptimekuma_monitor" "self" {
  id         = module.monitors.self_id
  depends_on = [module.monitors]
}

data "uptimekuma_tags" "all" {
  depends_on = [module.tags]
}

data "uptimekuma_tag" "environment" {
  name       = "environment"
  depends_on = [module.tags]
}

data "uptimekuma_notifications" "all" {
  depends_on = [module.notifications]
}

data "uptimekuma_maintenances" "all" {
  depends_on = [module.maintenance]
}

data "uptimekuma_status_page" "public" {
  slug       = module.status_page.slug
  depends_on = [module.status_page]
}

data "uptimekuma_status_pages" "all" {
  depends_on = [module.status_page]
}

data "uptimekuma_proxies" "all" {
  depends_on = [module.infrastructure]
}

data "uptimekuma_docker_hosts" "all" {
  depends_on = [module.infrastructure]
}

data "uptimekuma_api_keys" "all" {
  depends_on = [module.infrastructure]
}

data "uptimekuma_settings" "current" {
  depends_on = [module.settings]
}
