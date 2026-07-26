package maintenance

import (
	"context"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The Terraform model and the payload built from it.

type Model struct {
	ID          types.String `tfsdk:"id"`
	Title       types.String `tfsdk:"title"`
	Description types.String `tfsdk:"description"`
	Strategy    types.String `tfsdk:"strategy"`
	Active      types.Bool   `tfsdk:"active"`

	StartDate types.String `tfsdk:"start_date"`
	EndDate   types.String `tfsdk:"end_date"`
	StartTime types.String `tfsdk:"start_time"`
	EndTime   types.String `tfsdk:"end_time"`

	Weekdays    types.Set   `tfsdk:"weekdays"`
	DaysOfMonth types.Set   `tfsdk:"days_of_month"`
	IntervalDay types.Int64 `tfsdk:"interval_day"`

	Cron            types.String `tfsdk:"cron"`
	DurationMinutes types.Int64  `tfsdk:"duration_minutes"`
	Timezone        types.String `tfsdk:"timezone"`

	MonitorIDs    types.Set    `tfsdk:"monitor_ids"`
	StatusPageIDs types.Set    `tfsdk:"status_page_ids"`
	Status        types.String `tfsdk:"status"`
}

// wire converts the model into the API payload.
func (m *Model) wire(ctx context.Context, diags *diag.Diagnostics) kuma.Maintenance {
	maintenance := kuma.Maintenance{
		Title:          m.Title.ValueString(),
		Description:    m.Description.ValueString(),
		Strategy:       m.Strategy.ValueString(),
		Active:         common.BoolPtr(m.Active),
		IntervalDay:    common.IntPtr(m.IntervalDay),
		Cron:           common.StringPtr(m.Cron),
		TimezoneOption: common.StringPtr(m.Timezone),
	}

	// dateRange is always a two-element array; the server indexes into it
	// regardless of strategy.
	maintenance.DateRange = []*string{
		common.StringPtr(m.StartDate),
		common.StringPtr(m.EndDate),
	}

	if common.IsSet(m.StartTime) || common.IsSet(m.EndTime) {
		start, startOK := parseClockTime(m.StartTime, "start_time", diags)
		end, endOK := parseClockTime(m.EndTime, "end_time", diags)
		if startOK && endOK {
			maintenance.TimeRange = []kuma.TimePart{start, end}
		}
	}

	if common.IsSet(m.Weekdays) {
		maintenance.Weekdays = common.Int64SetToSlice(ctx, m.Weekdays)
	}
	if common.IsSet(m.DaysOfMonth) {
		days := common.Int64SetToSlice(ctx, m.DaysOfMonth)
		maintenance.DaysOfMonth = make([]any, 0, len(days))
		for _, day := range days {
			maintenance.DaysOfMonth = append(maintenance.DaysOfMonth, day)
		}
	}

	if common.IsSet(m.DurationMinutes) {
		duration := int(m.DurationMinutes.ValueInt64())
		maintenance.DurationMinutes = &duration
	}

	return maintenance
}
