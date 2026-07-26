# Every monitor type the provider supports, in two groups: the ones that watch
# this instance and go green with no internet access, and the ones that need
# outbound access.

# ── Groups ──────────────────────────────────────────────────────────

# A group runs no check of its own; children point at it with parent_id.
resource "uptimekuma_monitor_group" "internal" {
  name   = "Internal services"
  weight = 1000
}

resource "uptimekuma_monitor_group" "external" {
  name = "External dependencies"
}

# ── Works offline ───────────────────────────────────────────────────

# The port the container listens on, which is 3001 whatever KUMA_PORT maps it to
# on the host. The check runs inside the container, so the host port is no use.
resource "uptimekuma_monitor_http" "self" {
  name      = "Uptime Kuma itself"
  url       = "http://localhost:3001"
  interval  = 60
  parent_id = uptimekuma_monitor_group.internal.id

  notification_ids = compact([
    lookup(var.notification_ids, "slack", ""),
    lookup(var.notification_ids, "pagerduty", ""),
  ])

  tags = [
    {
      tag_id = var.tag_ids["environment"]
      value  = "demo"
    },
    {
      tag_id = var.tag_ids["team"]
      value  = "platform"
    },
  ]
}

resource "uptimekuma_monitor_port" "kuma_port" {
  name      = "Uptime Kuma port"
  hostname  = "localhost"
  port      = 3001
  interval  = 60
  parent_id = uptimekuma_monitor_group.internal.id
}

resource "uptimekuma_monitor_ping" "loopback" {
  name        = "Loopback"
  hostname    = "127.0.0.1"
  interval    = 60
  packet_size = 56
  parent_id   = uptimekuma_monitor_group.internal.id
}

# Watches the demo container through the socket mounted in docker-compose.yml.
resource "uptimekuma_monitor_docker" "kuma_container" {
  name           = "kuma-demo container"
  container_name = "kuma-demo"
  docker_host_id = var.docker_host_id
  interval       = 60
  parent_id      = uptimekuma_monitor_group.internal.id
}

# ── Needs outbound access ───────────────────────────────────────────

resource "uptimekuma_monitor_http" "external_api" {
  name                  = "example.com"
  url                   = "https://example.com"
  interval              = 300
  retry_interval        = 60
  max_retries           = 2
  description           = "An external target, so it needs outbound access"
  method                = "GET"
  accepted_status_codes = ["200-299"]
  ignore_tls            = false
  parent_id             = uptimekuma_monitor_group.external.id

  # Store the response body of failures, so notification templates can use
  # heartbeatJSON.response.
  save_error_response = true
  response_max_length = 4096

  notification_ids = compact([
    lookup(var.notification_ids, "webhook", ""),
    lookup(var.notification_ids, "smtp", ""),
    lookup(var.notification_ids, "discord", ""),
  ])
}

resource "uptimekuma_monitor_keyword" "keyword" {
  name           = "example.com says Example"
  url            = "https://example.com"
  keyword        = "Example"
  invert_keyword = false
  interval       = 300
  parent_id      = uptimekuma_monitor_group.external.id
}

resource "uptimekuma_monitor_json_query" "json_query" {
  name               = "GitHub API status"
  url                = "https://api.github.com"
  json_path          = "current_user_url"
  json_path_operator = "contains"
  expected_value     = "github.com"
  interval           = 300
  parent_id          = uptimekuma_monitor_group.external.id
}

resource "uptimekuma_monitor_dns" "dns" {
  name            = "example.com A record"
  hostname        = "example.com"
  resolver_server = "1.1.1.1"
  resolve_type    = "A"
  interval        = 300
  parent_id       = uptimekuma_monitor_group.external.id
}

# ── Passive and paused ──────────────────────────────────────────────

# A push monitor waits to be called instead of probing anything.
resource "uptimekuma_monitor_push" "nightly_job" {
  name      = "Nightly job heartbeat"
  interval  = 3600
  parent_id = uptimekuma_monitor_group.internal.id

  notification_ids = compact([
    lookup(var.notification_ids, "telegram", ""),
  ])
}

# Shows that `active` is applied through pause and resume, because Uptime Kuma's
# update event never writes that column.
resource "uptimekuma_monitor_http" "paused" {
  name   = "Paused on purpose"
  url    = "https://example.org"
  active = false
}
