package maintenance

import (
	"context"
	"strconv"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
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

// readInto fills the model from the server's representation.
func (m *Model) readInto(maintenance *kuma.Maintenance, monitorIDs, statusPageIDs []int, diags *diag.Diagnostics) {
	m.readIdentity(maintenance)
	m.readSchedule(maintenance, diags)
	m.readAssociations(monitorIDs, statusPageIDs, diags)
}

// readIdentity reads the fields that apply whatever the strategy is.
func (m *Model) readIdentity(maintenance *kuma.Maintenance) {
	m.ID = types.StringValue(strconv.Itoa(maintenance.ID))
	m.Title = types.StringValue(maintenance.Title)
	m.Description = types.StringValue(maintenance.Description)
	m.Strategy = types.StringValue(maintenance.Strategy)
	m.Active = common.BoolOrTrue(maintenance.Active)
	m.Status = types.StringValue(maintenance.Status)
}

// readSchedule reads the window itself: dates, times, recurrence and duration.
func (m *Model) readSchedule(maintenance *kuma.Maintenance, diags *diag.Diagnostics) {
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

	m.Weekdays = optionalInt64Set(maintenance.Weekdays, diags)
	m.DaysOfMonth = optionalInt64Set(daysOfMonth(maintenance.DaysOfMonth), diags)

	m.IntervalDay = common.IntValue(maintenance.IntervalDay)
	m.Cron = common.OptionalString(maintenance.Cron)
	m.DurationMinutes = durationMinutes(maintenance)

	if maintenance.TimezoneOption != nil && *maintenance.TimezoneOption != "" {
		m.Timezone = types.StringValue(*maintenance.TimezoneOption)
	} else {
		m.Timezone = types.StringValue("SAME_AS_SERVER")
	}
}

// readAssociations reads the monitors and status pages the window covers.
func (m *Model) readAssociations(monitorIDs, statusPageIDs []int, diags *diag.Diagnostics) {
	m.MonitorIDs = optionalInt64Set(monitorIDs, diags)
	m.StatusPageIDs = optionalInt64Set(statusPageIDs, diags)
}

// daysOfMonth narrows the any-typed field the server sends. JSON numbers decode
// as float64 through it.
func daysOfMonth(raw []any) []int {
	days := make([]int, 0, len(raw))
	for _, entry := range raw {
		switch value := entry.(type) {
		case float64:
			days = append(days, int(value))
		case int:
			days = append(days, value)
		}
	}
	return days
}

// durationMinutes reconciles the two fields the server answers with: reads report
// seconds in `duration`, writes take minutes.
func durationMinutes(maintenance *kuma.Maintenance) types.Int64 {
	switch {
	case maintenance.DurationMinutes != nil:
		return types.Int64Value(int64(*maintenance.DurationMinutes))
	case maintenance.Duration != nil:
		return types.Int64Value(int64(*maintenance.Duration / 60))
	default:
		return types.Int64Null()
	}
}

// optionalInt64Set is a null set when empty, so an absent list does not read back
// as an empty one and show up as a diff.
func optionalInt64Set(values []int, diags *diag.Diagnostics) types.Set {
	if len(values) == 0 {
		return types.SetNull(types.Int64Type)
	}
	return common.Int64Set(values, diags)
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
