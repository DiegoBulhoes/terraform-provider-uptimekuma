// Package resource aggregates the provider's managed resources.
//
// Each resource lives in its own subpackage so that a domain stays
// self-contained — the monitor package alone will grow to cover all 33 Uptime
// Kuma monitor types. This file is the single place that lists them, which keeps
// the provider from importing twenty packages just to register them.
package resource

import (
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/apikey"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/dockerhost"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/maintenance"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/monitor"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/notification"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/proxy"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/remotebrowser"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/settings"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/statuspage"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/statuspageincident"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/tag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// All returns every resource the provider registers.
func All() []func() resource.Resource {
	return []func() resource.Resource{
		// Monitors, one resource per Uptime Kuma monitor type.
		monitor.NewHTTPResource,
		monitor.NewKeywordResource,
		monitor.NewJSONQueryResource,
		monitor.NewPingResource,
		monitor.NewPortResource,
		monitor.NewDNSResource,
		monitor.NewPushResource,
		monitor.NewGroupResource,
		monitor.NewDockerResource,

		// Tags and notifications.
		tag.New,
		notification.New,

		// Maintenance windows.
		maintenance.New,

		// Status pages.
		statuspage.New,
		statuspageincident.New,

		// Infrastructure.
		proxy.New,
		dockerhost.New,
		remotebrowser.New,
		apikey.New,
		settings.New,
	}
}
