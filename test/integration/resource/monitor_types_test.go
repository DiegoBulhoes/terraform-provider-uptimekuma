//go:build integration

package resource_test

import (
	"testing"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/test/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccMonitorKeyword covers the keyword monitor, whose distinguishing feature
// is the body match on top of the HTTP request.
func TestAccMonitorKeyword(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkMonitorDestroyed("uptimekuma_monitor_keyword"),
		Steps: []resource.TestStep{
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_monitor_keyword" "test" {
  name           = "acc-keyword"
  url            = "https://example.com"
  keyword        = "Example"
  invert_keyword = false
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					monitorExists("uptimekuma_monitor_keyword.test"),
					resource.TestCheckResourceAttr("uptimekuma_monitor_keyword.test", "keyword", "Example"),
					resource.TestCheckResourceAttr("uptimekuma_monitor_keyword.test", "invert_keyword", "false"),
				),
			},
			{
				ResourceName:      "uptimekuma_monitor_keyword.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccMonitorJSONQuery covers the json-query monitor.
func TestAccMonitorJSONQuery(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkMonitorDestroyed("uptimekuma_monitor_json_query"),
		Steps: []resource.TestStep{
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_monitor_json_query" "test" {
  name               = "acc-json-query"
  url                = "https://example.com/health"
  json_path          = "status"
  json_path_operator = "=="
  expected_value     = "ok"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					monitorExists("uptimekuma_monitor_json_query.test"),
					resource.TestCheckResourceAttr("uptimekuma_monitor_json_query.test", "json_path", "status"),
					resource.TestCheckResourceAttr("uptimekuma_monitor_json_query.test", "expected_value", "ok"),
				),
			},
			{
				ResourceName:      "uptimekuma_monitor_json_query.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccMonitorPing covers the ping monitor and its ICMP-specific options.
func TestAccMonitorPing(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkMonitorDestroyed("uptimekuma_monitor_ping"),
		Steps: []resource.TestStep{
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_monitor_ping" "test" {
  name        = "acc-ping"
  hostname    = "127.0.0.1"
  packet_size = 56
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					monitorExists("uptimekuma_monitor_ping.test"),
					resource.TestCheckResourceAttr("uptimekuma_monitor_ping.test", "hostname", "127.0.0.1"),
					resource.TestCheckResourceAttr("uptimekuma_monitor_ping.test", "packet_size", "56"),
				),
			},
			{
				ResourceName:      "uptimekuma_monitor_ping.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccMonitorPort covers the TCP port monitor.
func TestAccMonitorPort(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkMonitorDestroyed("uptimekuma_monitor_port"),
		Steps: []resource.TestStep{
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_monitor_port" "test" {
  name     = "acc-port"
  hostname = "127.0.0.1"
  port     = 3001
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					monitorExists("uptimekuma_monitor_port.test"),
					resource.TestCheckResourceAttr("uptimekuma_monitor_port.test", "port", "3001"),
				),
			},
			{
				ResourceName:      "uptimekuma_monitor_port.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccMonitorDNS covers the DNS monitor.
func TestAccMonitorDNS(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkMonitorDestroyed("uptimekuma_monitor_dns"),
		Steps: []resource.TestStep{
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_monitor_dns" "test" {
  name            = "acc-dns"
  hostname        = "example.com"
  resolver_server = "1.1.1.1"
  resolve_type    = "A"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					monitorExists("uptimekuma_monitor_dns.test"),
					resource.TestCheckResourceAttr("uptimekuma_monitor_dns.test", "resolver_server", "1.1.1.1"),
					resource.TestCheckResourceAttr("uptimekuma_monitor_dns.test", "resolve_type", "A"),
				),
			},
			{
				ResourceName:      "uptimekuma_monitor_dns.test",
				ImportState:       true,
				ImportStateVerify: true,
				// last_result reflects the most recent check, so it can change
				// between the read and the import.
				ImportStateVerifyIgnore: []string{"last_result"},
			},
		},
	})
}

// TestAccMonitorPush covers the push monitor, including the server-generated
// token and the derived URL.
func TestAccMonitorPush(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkMonitorDestroyed("uptimekuma_monitor_push"),
		Steps: []resource.TestStep{
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_monitor_push" "test" {
  name     = "acc-push"
  interval = 60
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					monitorExists("uptimekuma_monitor_push.test"),
					resource.TestCheckResourceAttrSet("uptimekuma_monitor_push.test", "push_token"),
					resource.TestCheckResourceAttrSet("uptimekuma_monitor_push.test", "push_url"),
				),
			},
			{
				ResourceName:      "uptimekuma_monitor_push.test",
				ImportState:       true,
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccMonitorGroup covers the group monitor and the parent/child link, which
// is set on the child through parent_id.
func TestAccMonitorGroup(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkMonitorDestroyed("uptimekuma_monitor_group"),
		Steps: []resource.TestStep{
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_monitor_group" "parent" {
  name = "acc-group"
}

resource "uptimekuma_monitor_http" "child" {
  name      = "acc-group-child"
  url       = "https://example.com"
  parent_id = uptimekuma_monitor_group.parent.id
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					monitorExists("uptimekuma_monitor_group.parent"),
					monitorExists("uptimekuma_monitor_http.child"),
					resource.TestCheckResourceAttrPair(
						"uptimekuma_monitor_http.child", "parent_id",
						"uptimekuma_monitor_group.parent", "id",
					),
					// children_ids is deliberately not asserted here: the group
					// is created before its child, so its state was written when
					// the group still had no members.
				),
			},
			{
				// A refresh is what picks up the membership the child created.
				RefreshState: true,
				Check: resource.TestCheckResourceAttr(
					"uptimekuma_monitor_group.parent", "children_ids.#", "1",
				),
			},
		},
	})
}

// TestAccMonitorTags covers tag associations, which Uptime Kuma stores through
// separate events and which the provider reconciles after saving the monitor.
func TestAccMonitorTags(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		CheckDestroy:             checkMonitorDestroyed("uptimekuma_monitor_http"),
		Steps: []resource.TestStep{
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_tag" "env" {
  name  = "acc-env"
  color = "#4B5563"
}

resource "uptimekuma_monitor_http" "tagged" {
  name = "acc-tagged"
  url  = "https://example.com"

  tags = [
    {
      tag_id = uptimekuma_tag.env.id
      value  = "production"
    },
  ]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					monitorExists("uptimekuma_monitor_http.tagged"),
					resource.TestCheckResourceAttr("uptimekuma_monitor_http.tagged", "tags.#", "1"),
					resource.TestCheckResourceAttr("uptimekuma_monitor_http.tagged", "tags.0.value", "production"),
				),
			},
			{
				// Changing the value is a detach plus an attach, because the
				// value is part of the association's identity.
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_tag" "env" {
  name  = "acc-env"
  color = "#4B5563"
}

resource "uptimekuma_monitor_http" "tagged" {
  name = "acc-tagged"
  url  = "https://example.com"

  tags = [
    {
      tag_id = uptimekuma_tag.env.id
      value  = "staging"
    },
  ]
}
`,
				Check: resource.TestCheckResourceAttr("uptimekuma_monitor_http.tagged", "tags.0.value", "staging"),
			},
			{
				// Removing every tag must detach them, not leave them behind.
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_tag" "env" {
  name  = "acc-env"
  color = "#4B5563"
}

resource "uptimekuma_monitor_http" "tagged" {
  name = "acc-tagged"
  url  = "https://example.com"
}
`,
				Check: resource.TestCheckNoResourceAttr("uptimekuma_monitor_http.tagged", "tags.#"),
			},
		},
	})
}
