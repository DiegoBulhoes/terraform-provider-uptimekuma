//go:build integration

package resource_test

import (
	"testing"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/test/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccStatusPage covers the status page resource, including the group tree
// that only the HTTP route exposes.
func TestAccStatusPage(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_monitor_http" "one" {
  name = "acc-sp-one"
  url  = "https://example.com"
}

resource "uptimekuma_monitor_http" "two" {
  name = "acc-sp-two"
  url  = "https://example.org"
}

resource "uptimekuma_status_page" "test" {
  slug  = "acc-status"
  title = "Acceptance Status"

  description = "managed by terraform"
  theme       = "dark"
  show_tags   = true
  footer_text = "footer"

  group {
    name = "Core"

    monitor {
      monitor_id = uptimekuma_monitor_http.one.id
      send_url   = true
    }
  }

  group {
    name = "Secondary"

    monitor {
      monitor_id = uptimekuma_monitor_http.two.id
    }
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptimekuma_status_page.test", "id", "acc-status"),
					resource.TestCheckResourceAttr("uptimekuma_status_page.test", "slug", "acc-status"),
					resource.TestCheckResourceAttr("uptimekuma_status_page.test", "title", "Acceptance Status"),
					resource.TestCheckResourceAttr("uptimekuma_status_page.test", "theme", "dark"),
					resource.TestCheckResourceAttr("uptimekuma_status_page.test", "show_tags", "true"),
					// page_id is what maintenance windows reference.
					resource.TestCheckResourceAttrSet("uptimekuma_status_page.test", "page_id"),
					// Groups keep their configured order.
					resource.TestCheckResourceAttr("uptimekuma_status_page.test", "group.#", "2"),
					resource.TestCheckResourceAttr("uptimekuma_status_page.test", "group.0.name", "Core"),
					resource.TestCheckResourceAttr("uptimekuma_status_page.test", "group.1.name", "Secondary"),
					resource.TestCheckResourceAttr("uptimekuma_status_page.test", "group.0.monitor.0.send_url", "true"),
				),
			},
			{
				// Reordering groups and dropping one: the removed group must be
				// deleted, since the save call replaces the whole tree.
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_monitor_http" "one" {
  name = "acc-sp-one"
  url  = "https://example.com"
}

resource "uptimekuma_monitor_http" "two" {
  name = "acc-sp-two"
  url  = "https://example.org"
}

resource "uptimekuma_status_page" "test" {
  slug  = "acc-status"
  title = "Acceptance Status Renamed"
  theme = "light"

  group {
    name = "Only one left"

    monitor {
      monitor_id = uptimekuma_monitor_http.two.id
    }

    monitor {
      monitor_id = uptimekuma_monitor_http.one.id
    }
  }
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptimekuma_status_page.test", "title", "Acceptance Status Renamed"),
					resource.TestCheckResourceAttr("uptimekuma_status_page.test", "theme", "light"),
					resource.TestCheckResourceAttr("uptimekuma_status_page.test", "group.#", "1"),
					resource.TestCheckResourceAttr("uptimekuma_status_page.test", "group.0.name", "Only one left"),
					resource.TestCheckResourceAttr("uptimekuma_status_page.test", "group.0.monitor.#", "2"),
					// Monitor order is preserved as written.
					resource.TestCheckResourceAttrPair(
						"uptimekuma_status_page.test", "group.0.monitor.0.monitor_id",
						"uptimekuma_monitor_http.two", "id",
					),
				),
			},
			{
				ResourceName:      "uptimekuma_status_page.test",
				ImportState:       true,
				ImportStateId:     "acc-status",
				ImportStateVerify: true,
			},
		},
	})
}

// TestAccStatusPageIncident covers the incident banner and its pinned state.
func TestAccStatusPageIncident(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_status_page" "test" {
  slug  = "acc-incident-page"
  title = "Incident Page"
}

resource "uptimekuma_status_page_incident" "test" {
  status_page_slug = uptimekuma_status_page.test.slug
  title            = "Degraded performance"
  content          = "We are investigating."
  style            = "warning"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptimekuma_status_page_incident.test", "title", "Degraded performance"),
					resource.TestCheckResourceAttr("uptimekuma_status_page_incident.test", "style", "warning"),
					// Posting pins the incident.
					resource.TestCheckResourceAttr("uptimekuma_status_page_incident.test", "pinned", "true"),
					resource.TestCheckResourceAttr("uptimekuma_status_page_incident.test", "active", "true"),
					resource.TestMatchResourceAttr("uptimekuma_status_page_incident.test", "id",
						regexpIncidentID),
				),
			},
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_status_page" "test" {
  slug  = "acc-incident-page"
  title = "Incident Page"
}

resource "uptimekuma_status_page_incident" "test" {
  status_page_slug = uptimekuma_status_page.test.slug
  title            = "Still degraded"
  content          = "Working on a fix."
  style            = "danger"
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptimekuma_status_page_incident.test", "title", "Still degraded"),
					resource.TestCheckResourceAttr("uptimekuma_status_page_incident.test", "style", "danger"),
				),
			},
			{
				// pinned = false resolves the incident instead of deleting it.
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_status_page" "test" {
  slug  = "acc-incident-page"
  title = "Incident Page"
}

resource "uptimekuma_status_page_incident" "test" {
  status_page_slug = uptimekuma_status_page.test.slug
  title            = "Still degraded"
  content          = "Working on a fix."
  style            = "danger"
  pinned           = false
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptimekuma_status_page_incident.test", "pinned", "false"),
					resource.TestCheckResourceAttr("uptimekuma_status_page_incident.test", "active", "false"),
				),
			},
			{
				// Imported as <slug>/<incident id>, which is the composite ID.
				ResourceName:            "uptimekuma_status_page_incident.test",
				ImportState:             true,
				ImportStateVerify:       true,
				ImportStateVerifyIgnore: []string{"pinned"},
			},
		},
	})
}

// TestAccMaintenanceOnStatusPage covers the maintenance/status page link.
func TestAccMaintenanceOnStatusPage(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_status_page" "test" {
  slug  = "acc-maint-page"
  title = "Maintenance Page"
}

resource "uptimekuma_maintenance" "test" {
  title       = "acc-window-on-page"
  description = "shown on the status page"
  strategy    = "manual"

  status_page_ids = [uptimekuma_status_page.test.page_id]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("uptimekuma_maintenance.test", "status_page_ids.#", "1"),
					resource.TestCheckResourceAttrPair(
						"uptimekuma_maintenance.test", "status_page_ids.0",
						"uptimekuma_status_page.test", "page_id",
					),
				),
			},
			{
				// Clearing the set must detach the page.
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_status_page" "test" {
  slug  = "acc-maint-page"
  title = "Maintenance Page"
}

resource "uptimekuma_maintenance" "test" {
  title       = "acc-window-on-page"
  description = "shown on the status page"
  strategy    = "manual"
}
`,
				Check: resource.TestCheckNoResourceAttr("uptimekuma_maintenance.test", "status_page_ids.#"),
			},
		},
	})
}
