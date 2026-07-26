# Terraform Provider for Uptime Kuma

Manage [Uptime Kuma](https://github.com/louislam/uptime-kuma) 2.x with Terraform: monitors, tags, notification channels, maintenance windows, proxies, Docker hosts, remote browsers, API keys and instance settings.

Built with `terraform-plugin-framework` and a Socket.IO client written for this provider.

## Quick start

```hcl
terraform {
  required_providers {
    uptimekuma = {
      source = "DiegoBulhoes/uptimekuma"
    }
  }
}

provider "uptimekuma" {
  endpoint = "https://kuma.example.com"
  username = "admin"
  password = var.uptime_kuma_password
}

resource "uptimekuma_tag" "environment" {
  name  = "environment"
  color = "#4B5563"
}

resource "uptimekuma_notification" "slack" {
  name = "Slack #alerts"
  type = "slack"

  settings = jsonencode({
    slackwebhookURL = var.slack_webhook_url
    slackchannel    = "#alerts"
  })
}

resource "uptimekuma_monitor_http" "api" {
  name        = "API"
  url         = "https://api.example.com/health"
  interval    = 60
  max_retries = 3

  notification_ids = [uptimekuma_notification.slack.id]

  tags = [
    {
      tag_id = uptimekuma_tag.environment.id
      value  = "production"
    },
  ]
}
```

The provider also reads `UPTIME_KUMA_URL`, `UPTIME_KUMA_USERNAME`, `UPTIME_KUMA_PASSWORD` and `UPTIME_KUMA_TOKEN` from the environment.

## Try it locally

```bash
cd examples/demo && make demo
```

That starts Uptime Kuma in Docker and applies a configuration that uses every resource and data source, split into one module per area.

See [examples/demo](examples/demo).

## Resources

**Monitors** — one resource per Uptime Kuma monitor type, so each exposes only the attributes that apply to it:

`uptimekuma_monitor_http` · `uptimekuma_monitor_keyword` · `uptimekuma_monitor_json_query` · `uptimekuma_monitor_ping` · `uptimekuma_monitor_port` · `uptimekuma_monitor_dns` · `uptimekuma_monitor_push` · `uptimekuma_monitor_group` · `uptimekuma_monitor_docker`

**Status pages** — `uptimekuma_status_page` (page plus its groups of monitors) · `uptimekuma_status_page_incident`

**Everything else** — `uptimekuma_tag` · `uptimekuma_notification` · `uptimekuma_maintenance` · `uptimekuma_proxy` · `uptimekuma_docker_host` · `uptimekuma_remote_browser` · `uptimekuma_api_key` · `uptimekuma_settings`

## Data sources

`uptimekuma_monitor` · `uptimekuma_monitors` · `uptimekuma_tag` · `uptimekuma_tags` · `uptimekuma_notifications` · `uptimekuma_maintenances` · `uptimekuma_status_page` · `uptimekuma_status_pages` · `uptimekuma_proxies` · `uptimekuma_docker_hosts` · `uptimekuma_api_keys` · `uptimekuma_settings` · `uptimekuma_info`

## What to know before you use it

**Uptime Kuma has no REST API for writes.**

Its only HTTP endpoints are the read-only badge and status-page routes.

Everything else goes over Socket.IO, so the provider keeps a long-lived authenticated connection.

A few things follow from that:

- **Logins are limited to 20 per minute for the whole server.** Each Terraform command uses one. The provider retries with backoff and shares one session per configuration inside a process, but many workspaces running against a single instance in parallel can still hit the limit.

- **Some objects have no read event.** The server only pushes notifications, proxies, Docker hosts and remote browsers, so the provider keeps the pushed lists and reads from them.

- **Updates write the whole object, built from your configuration.** Attributes the provider does not model — the per-monitor condition tree from the web UI, for instance — are reset when a monitor is updated.

- **`active` is applied through pause and resume**, because Uptime Kuma's update event never writes that column.

- **Status pages are the exception to Socket.IO.** Their group tree is not returned by any event, so the provider reads it over HTTP from `/api/status-page/<slug>`.

- **A fresh 2.x instance needs its database chosen first.** On first boot the server stops at a database-selection screen, and the real API does not exist yet. Set `UPTIME_KUMA_DB_TYPE=sqlite` on the server, or finish that step in the UI, before you point Terraform at it.

**Uptime Kuma 1.x is not supported.**

Tested against 2.2, 2.3 and 2.4.

**API keys are not credentials for this provider.**

`uptimekuma_api_key` manages keys for the Prometheus `/metrics` endpoint.

The API this provider uses takes only a username and password.

## Development

```bash
make ci          # every check CI runs, without changing files
make build       # compile the provider
make test        # unit tests plus acceptance tests against Uptime Kuma 2.2-2.4
make docs        # regenerate docs/ from the schema, templates/ and examples/
make coverage    # unit plus acceptance coverage, merged; fails below 95%
```

Acceptance tests need Docker and Terraform on `PATH`.

They start an Uptime Kuma container per package and create the admin account themselves.

See [TESTING.md](TESTING.md).

Do not edit `docs/` by hand — it is generated.

Change `templates/` and `examples/`, then run `make docs`.

> **Note on the repository name.** The Terraform Registry takes a provider's name from its repository, and `tfplugindocs` takes it from the directory name. For the resource prefix to be `uptimekuma_`, the repository has to be named **`terraform-provider-uptimekuma`**. The Go module and the `make docs` targets already use that name.

## License

Apache 2.0 — see [LICENSE](LICENSE).
