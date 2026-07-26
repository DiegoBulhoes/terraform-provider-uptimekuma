package maintenance

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The schema. Which fields apply depends on the strategy.

func (r *Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A maintenance window. While it is active, the monitors it covers are shown as under " +
			"maintenance instead of down, and their notifications are suppressed.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Numeric ID of the maintenance window, assigned by Uptime Kuma.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"title": schema.StringAttribute{
				Description: "Title shown on status pages and in the dashboard.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "Description of the maintenance.",
				Required:    true,
			},
			"strategy": schema.StringAttribute{
				Description: "How the window is scheduled: `manual`, `single`, `cron`, `recurring-interval`, " +
					"`recurring-weekday` or `recurring-day-of-month`.",
				Required: true,
				Validators: []validator.String{
					stringvalidator.OneOf(
						StrategyManual,
						StrategySingle,
						StrategyCron,
						StrategyRecurringInterval,
						StrategyRecurringWeekday,
						StrategyRecurringDayOfMonth,
					),
				},
			},
			"active": schema.BoolAttribute{
				Description: "Whether the window is active. Set to false to pause it. Default: true.",
				Optional:    true,
				Computed:    true,
			},
			"start_date": schema.StringAttribute{
				Description: "Start of the window, for the `single` strategy and as the lower bound of recurring ones. Format: `2026-01-31 22:00`.",
				Optional:    true,
			},
			"end_date": schema.StringAttribute{
				Description: "End of the window, in the same format as `start_date`.",
				Optional:    true,
			},
			"start_time": schema.StringAttribute{
				Description: "Daily start time for recurring strategies, as `HH:MM`.",
				Optional:    true,
			},
			"end_time": schema.StringAttribute{
				Description: "Daily end time for recurring strategies, as `HH:MM`.",
				Optional:    true,
			},
			"weekdays": schema.SetAttribute{
				Description: "Days of the week for `recurring-weekday`, where 0 is Sunday.",
				Optional:    true,
				ElementType: types.Int64Type,
				Validators:  []validator.Set{},
			},
			"days_of_month": schema.SetAttribute{
				Description: "Days of the month for `recurring-day-of-month`, from 1 to 31.",
				Optional:    true,
				ElementType: types.Int64Type,
			},
			"interval_day": schema.Int64Attribute{
				Description: "Interval in days for `recurring-interval`.",
				Optional:    true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"cron": schema.StringAttribute{
				Description: "Cron expression for the `cron` strategy. For the recurring strategies the server " +
					"derives one from the schedule and reports it here.",
				Optional: true,
				Computed: true,
			},
			"duration_minutes": schema.Int64Attribute{
				Description: "How long the window lasts, in minutes. Required for the `cron` strategy.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"timezone": schema.StringAttribute{
				Description: "Timezone the schedule is interpreted in, or `SAME_AS_SERVER`. Default: `SAME_AS_SERVER`.",
				Optional:    true,
				Computed:    true,
			},
			"monitor_ids": schema.SetAttribute{
				Description: "IDs of the monitors this window covers. Uptime Kuma replaces the whole set on each save.",
				Optional:    true,
				ElementType: types.Int64Type,
			},
			"status_page_ids": schema.SetAttribute{
				Description: "Numeric IDs of the status pages that should show this window. Use the `page_id` " +
					"attribute of `uptimekuma_status_page`. Uptime Kuma replaces the whole set on each save.",
				Optional:    true,
				ElementType: types.Int64Type,
			},
			"status": schema.StringAttribute{
				Description: "Current state as computed by the server, for example `under-maintenance`, `scheduled` or `ended`.",
				Computed:    true,
			},
		},
	}
}
