package maintenance

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Maintenance strategies, exactly as the server names them.
const (
	StrategyManual              = "manual"
	StrategySingle              = "single"
	StrategyCron                = "cron"
	StrategyRecurringInterval   = "recurring-interval"
	StrategyRecurringWeekday    = "recurring-weekday"
	StrategyRecurringDayOfMonth = "recurring-day-of-month"
)

// Resource manages a maintenance window and the monitors it covers.
type Resource struct {
	client common.KumaClient
}

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

var (
	_ resource.Resource                = (*Resource)(nil)
	_ resource.ResourceWithConfigure   = (*Resource)(nil)
	_ resource.ResourceWithImportState = (*Resource)(nil)
)

func New() resource.Resource {
	return &Resource{}
}

func (r *Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_maintenance"
}

func (r *Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, err := common.ConfigureClient(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected provider data", err.Error())
		return
	}
	r.client = client
}

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

// readInto fills the model from the server's representation.
func (m *Model) readInto(maintenance *kuma.Maintenance, monitorIDs, statusPageIDs []int, diags *diag.Diagnostics) {
	m.ID = types.StringValue(strconv.Itoa(maintenance.ID))
	m.Title = types.StringValue(maintenance.Title)
	m.Description = types.StringValue(maintenance.Description)
	m.Strategy = types.StringValue(maintenance.Strategy)
	m.Active = common.BoolOrTrue(maintenance.Active)
	m.Status = types.StringValue(maintenance.Status)

	if len(maintenance.DateRange) > 0 {
		m.StartDate = common.StringValue(maintenance.DateRange[0])
	}
	if len(maintenance.DateRange) > 1 {
		m.EndDate = common.StringValue(maintenance.DateRange[1])
	}

	// Times only mean something for the recurring strategies; for the others the
	// server still returns midnight, which would show up as spurious state.
	if IsRecurring(maintenance.Strategy) && len(maintenance.TimeRange) == 2 {
		m.StartTime = types.StringValue(FormatClockTime(maintenance.TimeRange[0]))
		m.EndTime = types.StringValue(FormatClockTime(maintenance.TimeRange[1]))
	}

	if len(maintenance.Weekdays) > 0 {
		m.Weekdays = common.Int64Set(maintenance.Weekdays, diags)
	} else {
		m.Weekdays = types.SetNull(types.Int64Type)
	}

	if len(maintenance.DaysOfMonth) > 0 {
		days := make([]int, 0, len(maintenance.DaysOfMonth))
		for _, raw := range maintenance.DaysOfMonth {
			// JSON numbers decode as float64 through the any-typed field.
			switch value := raw.(type) {
			case float64:
				days = append(days, int(value))
			case int:
				days = append(days, value)
			}
		}
		m.DaysOfMonth = common.Int64Set(days, diags)
	} else {
		m.DaysOfMonth = types.SetNull(types.Int64Type)
	}

	m.IntervalDay = common.IntValue(maintenance.IntervalDay)
	m.Cron = common.OptionalString(maintenance.Cron)

	// Reads report seconds in `duration`; writes take minutes.
	switch {
	case maintenance.DurationMinutes != nil:
		m.DurationMinutes = types.Int64Value(int64(*maintenance.DurationMinutes))
	case maintenance.Duration != nil:
		m.DurationMinutes = types.Int64Value(int64(*maintenance.Duration / 60))
	default:
		m.DurationMinutes = types.Int64Null()
	}

	if maintenance.TimezoneOption != nil && *maintenance.TimezoneOption != "" {
		m.Timezone = types.StringValue(*maintenance.TimezoneOption)
	} else {
		m.Timezone = types.StringValue("SAME_AS_SERVER")
	}

	if len(monitorIDs) > 0 {
		m.MonitorIDs = common.Int64Set(monitorIDs, diags)
	} else {
		m.MonitorIDs = types.SetNull(types.Int64Type)
	}

	if len(statusPageIDs) > 0 {
		m.StatusPageIDs = common.Int64Set(statusPageIDs, diags)
	} else {
		m.StatusPageIDs = types.SetNull(types.Int64Type)
	}
}

// IsRecurring reports whether a strategy uses the daily time range. The server
// only reads timeRange, weekdays and daysOfMonth for these.
func IsRecurring(strategy string) bool {
	switch strategy {
	case StrategyRecurringInterval, StrategyRecurringWeekday, StrategyRecurringDayOfMonth:
		return true
	default:
		return false
	}
}

// ParseClockTime converts "HH:MM" into the hours/minutes object the API uses.
func ParseClockTime(value string) (kuma.TimePart, error) {
	var hours, minutes int
	// Sscanf alone is too permissive — it accepts "2:00 and then some" — so the
	// parsed values are rendered back and compared.
	if _, err := fmt.Sscanf(value, "%d:%d", &hours, &minutes); err != nil {
		return kuma.TimePart{}, fmt.Errorf("must be in HH:MM format, got %q", value)
	}
	if hours < 0 || hours > 23 || minutes < 0 || minutes > 59 {
		return kuma.TimePart{}, fmt.Errorf("must be a valid time of day, got %q", value)
	}
	part := kuma.TimePart{Hours: hours, Minutes: minutes}
	if FormatClockTime(part) != normalizeClockText(value) {
		return kuma.TimePart{}, fmt.Errorf("must be in HH:MM format, got %q", value)
	}
	return part, nil
}

// FormatClockTime renders a time part back as "HH:MM".
func FormatClockTime(part kuma.TimePart) string {
	return fmt.Sprintf("%02d:%02d", part.Hours, part.Minutes)
}

// normalizeClockText zero-pads a "H:M" input so it can be compared with the
// canonical rendering.
func normalizeClockText(value string) string {
	hours, minutes, found := strings.Cut(strings.TrimSpace(value), ":")
	if !found {
		return value
	}
	if len(hours) == 1 {
		hours = "0" + hours
	}
	if len(minutes) == 1 {
		minutes = "0" + minutes
	}
	return hours + ":" + minutes
}

// parseClockTime adapts ParseClockTime to Terraform diagnostics.
func parseClockTime(value types.String, attribute string, diags *diag.Diagnostics) (kuma.TimePart, bool) {
	if !common.IsSet(value) {
		return kuma.TimePart{}, true
	}
	part, err := ParseClockTime(value.ValueString())
	if err != nil {
		diags.AddError("Invalid time", fmt.Sprintf("`%s` %s.", attribute, err))
		return kuma.TimePart{}, false
	}
	return part, true
}

func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := model.wire(ctx, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	var id int
	err := common.RetryRPC(ctx, 3, func() error {
		var err error
		id, err = r.client.CreateMaintenance(ctx, payload)
		return err
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to create maintenance window", err.Error())
		return
	}

	monitorIDs := common.Int64SetToSlice(ctx, model.MonitorIDs)
	if len(monitorIDs) > 0 {
		if err := r.client.SetMaintenanceMonitors(ctx, id, monitorIDs); err != nil {
			resp.Diagnostics.AddError("Unable to attach monitors to the maintenance window", err.Error())
			return
		}
	}

	statusPageIDs := common.Int64SetToSlice(ctx, model.StatusPageIDs)
	if len(statusPageIDs) > 0 {
		if err := r.client.SetMaintenanceStatusPages(ctx, id, statusPageIDs); err != nil {
			resp.Diagnostics.AddError("Unable to attach status pages to the maintenance window", err.Error())
			return
		}
	}

	if !r.readInto(ctx, id, &model, &resp.Diagnostics) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model Model
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, ok := common.ParseID(model.ID, &resp.Diagnostics)
	if !ok {
		return
	}

	maintenance, err := r.client.GetMaintenance(ctx, id)
	if err != nil {
		if kuma.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read maintenance window", err.Error())
		return
	}

	monitorIDs, err := r.client.GetMaintenanceMonitors(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read the maintenance window's monitors", err.Error())
		return
	}
	statusPageIDs, err := r.client.GetMaintenanceStatusPages(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read the maintenance window's status pages", err.Error())
		return
	}

	model.readInto(maintenance, monitorIDs, statusPageIDs, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var model Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, ok := common.ParseID(model.ID, &resp.Diagnostics)
	if !ok {
		return
	}

	payload := model.wire(ctx, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	payload.ID = id

	err := common.RetryRPC(ctx, 3, func() error {
		return r.client.UpdateMaintenance(ctx, payload)
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to update maintenance window", err.Error())
		return
	}

	// Always sent, including when empty: the server replaces the whole set, so
	// this is also how an association is removed.
	if err := r.client.SetMaintenanceMonitors(ctx, id, common.Int64SetToSlice(ctx, model.MonitorIDs)); err != nil {
		resp.Diagnostics.AddError("Unable to update the maintenance window's monitors", err.Error())
		return
	}
	if err := r.client.SetMaintenanceStatusPages(ctx, id, common.Int64SetToSlice(ctx, model.StatusPageIDs)); err != nil {
		resp.Diagnostics.AddError("Unable to update the maintenance window's status pages", err.Error())
		return
	}

	if !r.readInto(ctx, id, &model, &resp.Diagnostics) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model Model
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, ok := common.ParseID(model.ID, &resp.Diagnostics)
	if !ok {
		return
	}

	err := common.RetryRPC(ctx, 3, func() error {
		return r.client.DeleteMaintenance(ctx, id)
	})
	if err != nil && !kuma.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete maintenance window", err.Error())
	}
}

func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

func (r *Resource) readInto(ctx context.Context, id int, model *Model, diags *diag.Diagnostics) bool {
	maintenance, err := r.client.GetMaintenance(ctx, id)
	if err != nil {
		diags.AddError("Unable to read the maintenance window back after saving", err.Error())
		return false
	}
	monitorIDs, err := r.client.GetMaintenanceMonitors(ctx, id)
	if err != nil {
		diags.AddError("Unable to read the maintenance window's monitors", err.Error())
		return false
	}
	statusPageIDs, err := r.client.GetMaintenanceStatusPages(ctx, id)
	if err != nil {
		diags.AddError("Unable to read the maintenance window's status pages", err.Error())
		return false
	}
	model.readInto(maintenance, monitorIDs, statusPageIDs, diags)
	return !diags.HasError()
}
