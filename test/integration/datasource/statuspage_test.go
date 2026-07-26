//go:build integration

package datasource_test

import (
	"testing"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/test/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestAccStatusPageDataSources covers both status page data sources. The single
// one reads its group tree over HTTP, which no other data source does.
func TestAccStatusPageDataSources(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_monitor_http" "target" {
  name = "acc-ds-sp-monitor"
  url  = "https://example.com"
}

resource "uptimekuma_status_page" "target" {
  slug        = "acc-ds-page"
  title       = "Data Source Page"
  description = "read by a data source"
  theme       = "dark"
  show_tags   = true

  group {
    name = "First"

    monitor {
      monitor_id = uptimekuma_monitor_http.target.id
      send_url   = true
    }
  }

  group {
    name = "Second"
  }
}

data "uptimekuma_status_page" "target" {
  slug       = uptimekuma_status_page.target.slug
  depends_on = [uptimekuma_status_page.target]
}

data "uptimekuma_status_pages" "all" {
  depends_on = [uptimekuma_status_page.target]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.uptimekuma_status_page.target", "title", "Data Source Page"),
					resource.TestCheckResourceAttr("data.uptimekuma_status_page.target", "theme", "dark"),
					resource.TestCheckResourceAttr("data.uptimekuma_status_page.target", "show_tags", "true"),
					resource.TestCheckResourceAttr("data.uptimekuma_status_page.target", "description", "read by a data source"),
					// page_id is what maintenance windows reference.
					resource.TestCheckResourceAttrPair(
						"data.uptimekuma_status_page.target", "page_id",
						"uptimekuma_status_page.target", "page_id",
					),
					// The group tree comes from the HTTP route, in display order.
					resource.TestCheckResourceAttr("data.uptimekuma_status_page.target", "groups.#", "2"),
					resource.TestCheckResourceAttr("data.uptimekuma_status_page.target", "groups.0.name", "First"),
					resource.TestCheckResourceAttr("data.uptimekuma_status_page.target", "groups.0.monitor_ids.#", "1"),
					resource.TestCheckResourceAttr("data.uptimekuma_status_page.target", "groups.1.name", "Second"),
					// The list data source reconnects to get a current list, so it
					// must see the page created in this same run.
					resource.TestCheckResourceAttr("data.uptimekuma_status_pages.all", "status_pages.#", "1"),
					resource.TestCheckResourceAttr("data.uptimekuma_status_pages.all", "status_pages.0.slug", "acc-ds-page"),
				),
			},
		},
	})
}

// TestAccMaintenanceAndAPIKeyDataSources covers the two remaining list data
// sources.
func TestAccMaintenanceAndAPIKeyDataSources(t *testing.T) {
	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { acctest.PreCheck(t) },
		ProtoV6ProviderFactories: acctest.ProviderFactories(),
		Steps: []resource.TestStep{
			{
				Config: acctest.ProviderConfig() + `
resource "uptimekuma_maintenance" "manual" {
  title       = "acc-ds-manual"
  description = "read by a data source"
  strategy    = "manual"
}

resource "uptimekuma_maintenance" "weekly" {
  title       = "acc-ds-weekly"
  description = "recurring"
  strategy    = "recurring-weekday"
  weekdays    = [2, 4]
  start_time  = "01:00"
  end_time    = "03:00"
}

resource "uptimekuma_api_key" "reader" {
  name = "acc-ds-key"
}

data "uptimekuma_maintenances" "all" {
  depends_on = [uptimekuma_maintenance.manual, uptimekuma_maintenance.weekly]
}

data "uptimekuma_api_keys" "all" {
  depends_on = [uptimekuma_api_key.reader]
}
`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.uptimekuma_maintenances.all", "maintenances.#", "2"),
					// Ordered by ID, so the manual one comes first.
					resource.TestCheckResourceAttr("data.uptimekuma_maintenances.all", "maintenances.0.title", "acc-ds-manual"),
					resource.TestCheckResourceAttr("data.uptimekuma_maintenances.all", "maintenances.0.strategy", "manual"),
					resource.TestCheckResourceAttrSet("data.uptimekuma_maintenances.all", "maintenances.0.status"),
					resource.TestCheckResourceAttr("data.uptimekuma_maintenances.all", "maintenances.1.strategy", "recurring-weekday"),

					resource.TestCheckResourceAttr("data.uptimekuma_api_keys.all", "api_keys.#", "1"),
					resource.TestCheckResourceAttr("data.uptimekuma_api_keys.all", "api_keys.0.name", "acc-ds-key"),
					resource.TestCheckResourceAttr("data.uptimekuma_api_keys.all", "api_keys.0.status", "active"),
					resource.TestCheckResourceAttr("data.uptimekuma_api_keys.all", "api_keys.0.active", "true"),
				),
			},
		},
	})
}
