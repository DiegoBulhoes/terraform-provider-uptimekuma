package statuspage

import (
	"context"
	"strings"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The Terraform models and the payloads built from them.

type Model struct {
	ID     types.String `tfsdk:"id"`
	PageID types.Int64  `tfsdk:"page_id"`
	Slug   types.String `tfsdk:"slug"`
	Title  types.String `tfsdk:"title"`

	Description types.String `tfsdk:"description"`
	Icon        types.String `tfsdk:"icon"`
	Theme       types.String `tfsdk:"theme"`
	FooterText  types.String `tfsdk:"footer_text"`
	CustomCSS   types.String `tfsdk:"custom_css"`
	RSSTitle    types.String `tfsdk:"rss_title"`

	AutoRefreshInterval types.Int64 `tfsdk:"auto_refresh_interval"`

	ShowTags              types.Bool `tfsdk:"show_tags"`
	ShowPoweredBy         types.Bool `tfsdk:"show_powered_by"`
	ShowCertificateExpiry types.Bool `tfsdk:"show_certificate_expiry"`
	ShowOnlyLastHeartbeat types.Bool `tfsdk:"show_only_last_heartbeat"`

	AnalyticsID        types.String `tfsdk:"analytics_id"`
	AnalyticsScriptURL types.String `tfsdk:"analytics_script_url"`
	AnalyticsType      types.String `tfsdk:"analytics_type"`

	DomainNameList types.List `tfsdk:"domain_names"`

	Published types.Bool `tfsdk:"published"`

	Groups []GroupModel `tfsdk:"group"`
}

// GroupModel is one section of the page. Declared as a block so the order in the
// configuration is the order on the page.
type GroupModel struct {
	Name     types.String   `tfsdk:"name"`
	Monitors []MonitorModel `tfsdk:"monitor"`
}

// MonitorModel is a monitor inside a group.
type MonitorModel struct {
	MonitorID types.Int64  `tfsdk:"monitor_id"`
	SendURL   types.Bool   `tfsdk:"send_url"`
	URL       types.String `tfsdk:"url"`
}

// config turns the model into the API payload, leaving the icon out: that goes
// in saveStatusPage's separate imgDataUrl argument.
func (m *Model) config(ctx context.Context, diags *diag.Diagnostics) kuma.StatusPage {
	page := kuma.StatusPage{
		Slug:                  strings.ToLower(m.Slug.ValueString()),
		Title:                 m.Title.ValueString(),
		Description:           common.StringPtr(m.Description),
		Theme:                 m.Theme.ValueString(),
		AutoRefreshInterval:   common.IntPtr(m.AutoRefreshInterval),
		ShowTags:              common.BoolPtr(m.ShowTags),
		ShowPoweredBy:         common.BoolPtr(m.ShowPoweredBy),
		ShowCertificateExpiry: common.BoolPtr(m.ShowCertificateExpiry),
		ShowOnlyLastHeartbeat: common.BoolPtr(m.ShowOnlyLastHeartbeat),
		CustomCSS:             common.StringPtr(m.CustomCSS),
		FooterText:            common.StringPtr(m.FooterText),
		RSSTitle:              common.StringPtr(m.RSSTitle),
		AnalyticsID:           common.StringPtr(m.AnalyticsID),
		AnalyticsScriptURL:    common.StringPtr(m.AnalyticsScriptURL),
		AnalyticsType:         common.StringPtr(m.AnalyticsType),
		DomainNameList:        common.StringListToSlice(ctx, m.DomainNameList),
	}
	if page.DomainNameList == nil {
		page.DomainNameList = []string{}
	}
	_ = diags
	return page
}

// groups turns the configured blocks into the payload, in order.
func (m *Model) groups() []kuma.StatusPageGroup {
	groups := make([]kuma.StatusPageGroup, 0, len(m.Groups))
	for _, group := range m.Groups {
		monitors := make([]kuma.StatusPageMonitor, 0, len(group.Monitors))
		for _, monitor := range group.Monitors {
			monitors = append(monitors, kuma.StatusPageMonitor{
				ID:      int(monitor.MonitorID.ValueInt64()),
				SendURL: common.BoolPtr(monitor.SendURL),
				URL:     common.StringPtr(monitor.URL),
			})
		}
		groups = append(groups, kuma.StatusPageGroup{
			Name:        group.Name.ValueString(),
			MonitorList: monitors,
		})
	}
	return groups
}

// icon picks the value for the imgDataUrl argument.
func (m *Model) icon() string {
	if common.IsSet(m.Icon) {
		return m.Icon.ValueString()
	}
	return defaultIcon
}
