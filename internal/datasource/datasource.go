// Package datasource aggregates the provider's data sources.
//
// The layout mirrors internal/resource: one subpackage per entity, listed here
// so the provider imports a single package to register them all.
package datasource

import (
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/datasource/apikey"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/datasource/dockerhost"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/datasource/info"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/datasource/maintenance"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/datasource/monitor"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/datasource/notification"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/datasource/proxy"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/datasource/settings"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/datasource/statuspage"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/datasource/tag"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
)

// All returns every data source the provider registers.
func All() []func() datasource.DataSource {
	return []func() datasource.DataSource{
		monitor.New,
		monitor.NewList,
		tag.New,
		tag.NewList,
		notification.NewList,
		maintenance.NewList,
		statuspage.New,
		statuspage.NewList,
		proxy.NewList,
		dockerhost.NewList,
		apikey.NewList,
		settings.New,
		info.New,
	}
}
