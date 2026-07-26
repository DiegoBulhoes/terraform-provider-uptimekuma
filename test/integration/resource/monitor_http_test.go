//go:build integration

package resource_test

import (
	"testing"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/test/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccMonitorHTTP covers the full lifecycle of the HTTP monitor resource:
// create, in-place update, import and destroy.
func TestAccMonitorHTTP(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkMonitorDestroyed("uptimekuma_monitor_http"),
		Steps: []resource.TestStep{
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_monitor_http" "test" {
  name     = "acc-http"
  url      = "https://example.com"
  interval = 120
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					monitorExists("uptimekuma_monitor_http.test"),
					resource.TestCheckResourceAttr("uptimekuma_monitor_http.test", "name", "acc-http"),
					resource.TestCheckResourceAttr("uptimekuma_monitor_http.test", "url", "https://example.com"),
					resource.TestCheckResourceAttr("uptimekuma_monitor_http.test", "interval", "120"),
					// The client mirrors the check interval when no retry
					// interval is given, matching the web UI.
					resource.TestCheckResourceAttr("uptimekuma_monitor_http.test", "retry_interval", "120"),
					// Server-side defaults must land in state, not stay unknown.
					resource.TestCheckResourceAttr("uptimekuma_monitor_http.test", "method", "GET"),
					resource.TestCheckResourceAttr("uptimekuma_monitor_http.test", "active", "true"),
					resource.TestCheckResourceAttr("uptimekuma_monitor_http.test", "accepted_status_codes.#", "1"),
					resource.TestCheckResourceAttr("uptimekuma_monitor_http.test", "accepted_status_codes.0", "200-299"),
					resource.TestCheckResourceAttrSet("uptimekuma_monitor_http.test", "id"),
				),
			},
			{
				// Update in place: editMonitor is a whole-object write, so this
				// also proves the read-modify-write does not drop other fields.
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_monitor_http" "test" {
  name                  = "acc-http-renamed"
  url                   = "https://example.org"
  interval              = 300
  retry_interval        = 60
  max_retries           = 3
  description           = "managed by terraform"
  method                = "POST"
  body                  = "{\"ping\":true}"
  accepted_status_codes = ["200-299", "404"]
  ignore_tls            = true
  upside_down           = false
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					monitorExists("uptimekuma_monitor_http.test"),
					resource.TestCheckResourceAttr("uptimekuma_monitor_http.test", "name", "acc-http-renamed"),
					resource.TestCheckResourceAttr("uptimekuma_monitor_http.test", "url", "https://example.org"),
					resource.TestCheckResourceAttr("uptimekuma_monitor_http.test", "interval", "300"),
					resource.TestCheckResourceAttr("uptimekuma_monitor_http.test", "retry_interval", "60"),
					resource.TestCheckResourceAttr("uptimekuma_monitor_http.test", "max_retries", "3"),
					resource.TestCheckResourceAttr("uptimekuma_monitor_http.test", "description", "managed by terraform"),
					resource.TestCheckResourceAttr("uptimekuma_monitor_http.test", "method", "POST"),
					resource.TestCheckResourceAttr("uptimekuma_monitor_http.test", "ignore_tls", "true"),
					resource.TestCheckResourceAttr("uptimekuma_monitor_http.test", "accepted_status_codes.#", "2"),
				),
			},
			{
				ResourceName:      "uptimekuma_monitor_http.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccMonitorHTTPResponseSaving covers the response-body options from the
// monitor's Advanced section, including the one whose server-side default is
// true rather than false.
func TestAccMonitorHTTPResponseSaving(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkMonitorDestroyed("uptimekuma_monitor_http"),
		Steps: []resource.TestStep{
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_monitor_http" "response" {
  name = "acc-http-response"
  url  = "https://example.com"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					monitorExists("uptimekuma_monitor_http.response"),
					// Server defaults: off for successes, on for errors, 1 KiB.
					resource.TestCheckResourceAttr("uptimekuma_monitor_http.response", "save_response", "false"),
					resource.TestCheckResourceAttr("uptimekuma_monitor_http.response", "save_error_response", "true"),
					resource.TestCheckResourceAttr("uptimekuma_monitor_http.response", "response_max_length", "1024"),
				),
			},
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_monitor_http" "response" {
  name                = "acc-http-response"
  url                 = "https://example.com"
  save_response       = true
  save_error_response = false
  response_max_length = 8192
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptimekuma_monitor_http.response", "save_response", "true"),
					resource.TestCheckResourceAttr("uptimekuma_monitor_http.response", "save_error_response", "false"),
					resource.TestCheckResourceAttr("uptimekuma_monitor_http.response", "response_max_length", "8192"),
				),
			},
			{
				ResourceName:      "uptimekuma_monitor_http.response",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccMonitorHTTPPause checks that toggling `active` pauses and resumes the
// monitor rather than recreating it.
func TestAccMonitorHTTPPause(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkMonitorDestroyed("uptimekuma_monitor_http"),
		Steps: []resource.TestStep{
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_monitor_http" "paused" {
  name   = "acc-http-paused"
  url    = "https://example.com"
  active = false
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					monitorExists("uptimekuma_monitor_http.paused"),
					resource.TestCheckResourceAttr("uptimekuma_monitor_http.paused", "active", "false"),
				),
			},
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_monitor_http" "paused" {
  name   = "acc-http-paused"
  url    = "https://example.com"
  active = true
}
`,
				Check: resource.TestCheckResourceAttr("uptimekuma_monitor_http.paused", "active", "true"),
			},
		},
	})
}
