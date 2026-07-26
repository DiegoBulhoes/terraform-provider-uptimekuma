//go:build integration

// Package datasource_test holds the acceptance tests for the provider's data
// sources.
package datasource_test

import (
	"testing"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/test/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

func TestMain(m *testing.M) {
	acctest.SetupTestContainer(m)
}

// TestAccMonitorDataSource looks a monitor up both ways: by ID and by name.
func TestAccMonitorDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_monitor_http" "target" {
  name     = "acc-ds-target"
  url      = "https://example.com"
  interval = 90
}

data "uptimekuma_monitor" "by_id" {
  id = uptimekuma_monitor_http.target.id
}

data "uptimekuma_monitor" "by_name" {
  name       = uptimekuma_monitor_http.target.name
  depends_on = [uptimekuma_monitor_http.target]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.uptimekuma_monitor.by_id", "name", "acc-ds-target"),
					resource.TestCheckResourceAttr("data.uptimekuma_monitor.by_id", "type", "http"),
					resource.TestCheckResourceAttr("data.uptimekuma_monitor.by_id", "url", "https://example.com"),
					resource.TestCheckResourceAttr("data.uptimekuma_monitor.by_id", "interval", "90"),
					// Both lookups must resolve to the same monitor.
					resource.TestCheckResourceAttrPair(
						"data.uptimekuma_monitor.by_name", "id",
						"uptimekuma_monitor_http.target", "id",
					),
				),
			},
		},
	})
}

// TestAccMonitorDataSourceErrors covers the mutually exclusive lookup arguments.
func TestAccMonitorDataSourceErrors(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: acctest.ProviderConfig() + `
data "uptimekuma_monitor" "neither" {}
`,
				ExpectError: regexpMissingLookup,
			},
			{
				Config: acctest.ProviderConfig() + `
data "uptimekuma_monitor" "both" {
  id   = "1"
  name = "whatever"
}
`,
				ExpectError: regexpAmbiguousLookup,
			},
			{
				Config: acctest.ProviderConfig() + `
data "uptimekuma_monitor" "missing" {
  name = "no-such-monitor-anywhere"
}
`,
				ExpectError: regexpNotFound,
			},
		},
	})
}

// TestAccMonitorsDataSource covers the list data source and its type filter.
func TestAccMonitorsDataSource(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_monitor_http" "one" {
  name = "acc-ds-list-http"
  url  = "https://example.com"
}

resource "uptimekuma_monitor_ping" "two" {
  name     = "acc-ds-list-ping"
  hostname = "127.0.0.1"
}

data "uptimekuma_monitors" "http_only" {
  type       = "http"
  depends_on = [uptimekuma_monitor_http.one, uptimekuma_monitor_ping.two]
}

data "uptimekuma_monitors" "all" {
  depends_on = [uptimekuma_monitor_http.one, uptimekuma_monitor_ping.two]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.uptimekuma_monitors.http_only", "monitors.#", "1"),
					resource.TestCheckResourceAttr("data.uptimekuma_monitors.http_only", "monitors.0.type", "http"),
					resource.TestCheckResourceAttr("data.uptimekuma_monitors.all", "monitors.#", "2"),
					resource.TestCheckResourceAttr("data.uptimekuma_monitors.all", "ids.#", "2"),
				),
			},
		},
	})
}

// TestAccTagDataSources covers the tag and tags data sources.
func TestAccTagDataSources(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_tag" "target" {
  name  = "acc-ds-tag"
  color = "#059669"
}

data "uptimekuma_tag" "by_name" {
  name       = uptimekuma_tag.target.name
  depends_on = [uptimekuma_tag.target]
}

data "uptimekuma_tags" "all" {
  depends_on = [uptimekuma_tag.target]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.uptimekuma_tag.by_name", "color", "#059669"),
					resource.TestCheckResourceAttrPair(
						"data.uptimekuma_tag.by_name", "id",
						"uptimekuma_tag.target", "id",
					),
					resource.TestCheckResourceAttr("data.uptimekuma_tags.all", "tags.#", "1"),
				),
			},
		},
	})
}

// TestAccInfraDataSources covers the supporting listings plus the server info,
// which is the only place the provider surfaces the server version.
func TestAccInfraDataSources(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_proxy" "target" {
  protocol = "http"
  host     = "proxy.example.com"
  port     = 3128
}

resource "uptimekuma_docker_host" "target" {
  name            = "acc-ds-docker"
  connection_type = "socket"
  daemon          = "/var/run/docker.sock"
}

resource "uptimekuma_notification" "target" {
  name     = "acc-ds-notification"
  type     = "webhook"
  settings = jsonencode({ webhookURL = "https://example.com/hook", webhookContentType = "json" })
}

data "uptimekuma_proxies" "all" {
  depends_on = [uptimekuma_proxy.target]
}

data "uptimekuma_docker_hosts" "all" {
  depends_on = [uptimekuma_docker_host.target]
}

data "uptimekuma_notifications" "all" {
  depends_on = [uptimekuma_notification.target]
}

data "uptimekuma_settings" "current" {}

data "uptimekuma_info" "server" {}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.uptimekuma_proxies.all", "proxies.#", "1"),
					resource.TestCheckResourceAttr("data.uptimekuma_proxies.all", "proxies.0.host", "proxy.example.com"),
					resource.TestCheckResourceAttr("data.uptimekuma_docker_hosts.all", "docker_hosts.#", "1"),
					resource.TestCheckResourceAttr("data.uptimekuma_notifications.all", "notifications.#", "1"),
					resource.TestCheckResourceAttr("data.uptimekuma_notifications.all", "notifications.0.type", "webhook"),
					resource.TestCheckResourceAttrSet("data.uptimekuma_settings.current", "settings"),
					resource.TestCheckResourceAttrSet("data.uptimekuma_info.server", "version"),
					// The tests run against a container, so this must be true.
					resource.TestCheckResourceAttr("data.uptimekuma_info.server", "is_container", "true"),
				),
			},
		},
	})
}
