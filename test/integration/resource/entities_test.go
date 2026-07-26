//go:build integration

package resource_test

import (
	"testing"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/test/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccTag covers the tag resource.
func TestAccTag(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_tag" "test" {
  name  = "acc-tag"
  color = "#4B5563"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptimekuma_tag.test", "name", "acc-tag"),
					resource.TestCheckResourceAttr("uptimekuma_tag.test", "color", "#4B5563"),
					resource.TestCheckResourceAttrSet("uptimekuma_tag.test", "id"),
				),
			},
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_tag" "test" {
  name  = "acc-tag-renamed"
  color = "#DC2626"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptimekuma_tag.test", "name", "acc-tag-renamed"),
					resource.TestCheckResourceAttr("uptimekuma_tag.test", "color", "#DC2626"),
				),
			},
			{
				ResourceName:      "uptimekuma_tag.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccNotification covers the notification resource, whose provider-specific
// options live in a JSON attribute.
func TestAccNotification(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_notification" "test" {
  name = "acc-webhook"
  type = "webhook"

  settings = jsonencode({
    webhookURL         = "https://example.com/hook"
    webhookContentType = "json"
  })
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptimekuma_notification.test", "name", "acc-webhook"),
					resource.TestCheckResourceAttr("uptimekuma_notification.test", "type", "webhook"),
					resource.TestCheckResourceAttr("uptimekuma_notification.test", "is_default", "false"),
					resource.TestCheckResourceAttrSet("uptimekuma_notification.test", "settings"),
				),
			},
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_notification" "test" {
  name       = "acc-webhook-renamed"
  type       = "webhook"
  is_default = true

  settings = jsonencode({
    webhookURL         = "https://example.com/hook2"
    webhookContentType = "form-data"
  })
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptimekuma_notification.test", "name", "acc-webhook-renamed"),
					resource.TestCheckResourceAttr("uptimekuma_notification.test", "is_default", "true"),
				),
			},
			{
				ResourceName:      "uptimekuma_notification.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccNotificationAttachedToMonitor checks the monitor/notification link,
// which the API models as a set encoded in an object.
func TestAccNotificationAttachedToMonitor(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkMonitorDestroyed("uptimekuma_monitor_http"),
		Steps: []resource.TestStep{
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_notification" "hook" {
  name     = "acc-attached-hook"
  type     = "webhook"
  settings = jsonencode({ webhookURL = "https://example.com/hook", webhookContentType = "json" })
}

resource "uptimekuma_monitor_http" "notified" {
  name             = "acc-notified"
  url              = "https://example.com"
  notification_ids = [uptimekuma_notification.hook.id]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					monitorExists("uptimekuma_monitor_http.notified"),
					resource.TestCheckResourceAttr("uptimekuma_monitor_http.notified", "notification_ids.#", "1"),
				),
			},
			{
				// Detaching must clear the association.
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_notification" "hook" {
  name     = "acc-attached-hook"
  type     = "webhook"
  settings = jsonencode({ webhookURL = "https://example.com/hook", webhookContentType = "json" })
}

resource "uptimekuma_monitor_http" "notified" {
  name = "acc-notified"
  url  = "https://example.com"
}
`,
				Check: resource.TestCheckNoResourceAttr("uptimekuma_monitor_http.notified", "notification_ids.#"),
			},
		},
	})
}

// TestAccMaintenanceManual covers the manual strategy and the monitor
// association, which the server replaces wholesale on each save.
func TestAccMaintenanceManual(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_monitor_http" "covered" {
  name = "acc-maint-covered"
  url  = "https://example.com"
}

resource "uptimekuma_maintenance" "test" {
  title       = "acc-maintenance"
  description = "managed by terraform"
  strategy    = "manual"
  monitor_ids = [uptimekuma_monitor_http.covered.id]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptimekuma_maintenance.test", "title", "acc-maintenance"),
					resource.TestCheckResourceAttr("uptimekuma_maintenance.test", "strategy", "manual"),
					resource.TestCheckResourceAttr("uptimekuma_maintenance.test", "monitor_ids.#", "1"),
					resource.TestCheckResourceAttrSet("uptimekuma_maintenance.test", "status"),
				),
			},
			{
				// Clearing monitor_ids must detach every monitor.
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_monitor_http" "covered" {
  name = "acc-maint-covered"
  url  = "https://example.com"
}

resource "uptimekuma_maintenance" "test" {
  title       = "acc-maintenance-renamed"
  description = "managed by terraform"
  strategy    = "manual"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptimekuma_maintenance.test", "title", "acc-maintenance-renamed"),
					resource.TestCheckNoResourceAttr("uptimekuma_maintenance.test", "monitor_ids.#"),
				),
			},
			{
				ResourceName:      "uptimekuma_maintenance.test",
				ImportState:       true,
				ImportStateVerify: true,
				// The server recomputes status from the schedule, so it can move
				// between the read and the import.
				ImportStateVerifyIgnore: []string{"status"},
			},
		},
	})
}

// TestAccMaintenanceRecurring covers a recurring strategy, where the time range
// and weekday list are what the server actually reads.
func TestAccMaintenanceRecurring(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_maintenance" "weekly" {
  title       = "acc-weekly"
  description = "weekly window"
  strategy    = "recurring-weekday"
  weekdays    = [1, 3, 5]
  start_time  = "02:00"
  end_time    = "04:30"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptimekuma_maintenance.weekly", "strategy", "recurring-weekday"),
					resource.TestCheckResourceAttr("uptimekuma_maintenance.weekly", "weekdays.#", "3"),
					resource.TestCheckResourceAttr("uptimekuma_maintenance.weekly", "start_time", "02:00"),
					resource.TestCheckResourceAttr("uptimekuma_maintenance.weekly", "end_time", "04:30"),
				),
			},
		},
	})
}

// TestAccProxy covers the proxy resource.
func TestAccProxy(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_proxy" "test" {
  protocol = "http"
  host     = "proxy.example.com"
  port     = 3128
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptimekuma_proxy.test", "protocol", "http"),
					resource.TestCheckResourceAttr("uptimekuma_proxy.test", "port", "3128"),
					resource.TestCheckResourceAttr("uptimekuma_proxy.test", "active", "true"),
				),
			},
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_proxy" "test" {
  protocol = "socks5"
  host     = "proxy.example.com"
  port     = 1080
  username = "proxyuser"
  password = "proxypass"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptimekuma_proxy.test", "protocol", "socks5"),
					resource.TestCheckResourceAttr("uptimekuma_proxy.test", "port", "1080"),
					resource.TestCheckResourceAttr("uptimekuma_proxy.test", "username", "proxyuser"),
				),
			},
			{
				ResourceName:      "uptimekuma_proxy.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccDockerHost covers the docker host resource and a docker monitor that
// depends on it.
func TestAccDockerHost(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkMonitorDestroyed("uptimekuma_monitor_docker"),
		Steps: []resource.TestStep{
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_docker_host" "test" {
  name            = "acc-docker"
  connection_type = "socket"
  daemon          = "/var/run/docker.sock"
}

resource "uptimekuma_monitor_docker" "test" {
  name           = "acc-docker-monitor"
  container_name = "some-container"
  docker_host_id = uptimekuma_docker_host.test.id
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptimekuma_docker_host.test", "name", "acc-docker"),
					resource.TestCheckResourceAttr("uptimekuma_docker_host.test", "connection_type", "socket"),
					monitorExists("uptimekuma_monitor_docker.test"),
					resource.TestCheckResourceAttrPair(
						"uptimekuma_monitor_docker.test", "docker_host_id",
						"uptimekuma_docker_host.test", "id",
					),
				),
			},
			{
				// Switching connection type and daemon in place.
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_docker_host" "test" {
  name            = "acc-docker-renamed"
  connection_type = "tcp"
  daemon          = "tcp://docker.example.com:2375"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptimekuma_docker_host.test", "name", "acc-docker-renamed"),
					resource.TestCheckResourceAttr("uptimekuma_docker_host.test", "connection_type", "tcp"),
					resource.TestCheckResourceAttr("uptimekuma_docker_host.test", "daemon", "tcp://docker.example.com:2375"),
				),
			},
			{
				ResourceName:      "uptimekuma_docker_host.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccRemoteBrowser covers the remote browser resource.
func TestAccRemoteBrowser(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_remote_browser" "test" {
  name = "acc-browser"
  url  = "ws://chrome:3000"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptimekuma_remote_browser.test", "name", "acc-browser"),
					resource.TestCheckResourceAttr("uptimekuma_remote_browser.test", "url", "ws://chrome:3000"),
				),
			},
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_remote_browser" "test" {
  name = "acc-browser-renamed"
  url  = "ws://chrome-2.example.com:3000"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptimekuma_remote_browser.test", "name", "acc-browser-renamed"),
					resource.TestCheckResourceAttr("uptimekuma_remote_browser.test", "url", "ws://chrome-2.example.com:3000"),
				),
			},
			{
				ResourceName:      "uptimekuma_remote_browser.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccAPIKey covers the API key resource, including the write-once secret and
// the enable/disable path.
func TestAccAPIKey(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_api_key" "test" {
  name = "acc-key"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptimekuma_api_key.test", "name", "acc-key"),
					resource.TestCheckResourceAttr("uptimekuma_api_key.test", "active", "true"),
					// The clear-text key is only ever returned at creation.
					resource.TestCheckResourceAttrSet("uptimekuma_api_key.test", "key"),
					resource.TestCheckResourceAttr("uptimekuma_api_key.test", "status", "active"),
				),
			},
			{
				// active is the only in-place change the API supports.
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_api_key" "test" {
  name   = "acc-key"
  active = false
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptimekuma_api_key.test", "active", "false"),
					resource.TestCheckResourceAttr("uptimekuma_api_key.test", "status", "inactive"),
					resource.TestCheckResourceAttrSet("uptimekuma_api_key.test", "key"),
				),
			},
			{
				ResourceName:      "uptimekuma_api_key.test",
				ImportState:       true,
				ImportStateVerify: true,
				// The clear-text key exists only in the creation response, so an
				// imported key cannot have it.
				ImportStateVerifyIgnore: []string{"key"},
			},
		},
	})
}

// TestAccSettings covers the singleton settings resource.
func TestAccSettings(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_settings" "test" {
  settings = jsonencode({
    keepDataPeriodDays = 190
  })
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptimekuma_settings.test", "id", "settings"),
					resource.TestCheckResourceAttrSet("uptimekuma_settings.test", "all"),
				),
			},
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_settings" "test" {
  settings = jsonencode({
    keepDataPeriodDays = 200
  })
}
`,
				Check: resource.TestCheckResourceAttrSet("uptimekuma_settings.test", "all"),
			},
		},
	})
}
