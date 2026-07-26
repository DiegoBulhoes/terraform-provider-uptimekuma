// Package monitor holds one resource per Uptime Kuma monitor type. The whole
// CRUD cycle lives in resource.go; each type file only declares its own
// attributes and how they map onto the wire payload.
package monitor

import (
	"context"
	"fmt"
	"strconv"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Uptime Kuma applies no defaults of its own for these, and validate() rejects
// intervals below the minimum, so the provider supplies them.
const (
	defaultInterval       = 60
	defaultResendInterval = 0
	defaultMaxRetries     = 0
)

// BaseModel holds the attributes every monitor type shares. Each
// per-type model embeds it, and the framework promotes the fields.
type BaseModel struct {
	ID              types.String  `tfsdk:"id"`
	Name            types.String  `tfsdk:"name"`
	Description     types.String  `tfsdk:"description"`
	Active          types.Bool    `tfsdk:"active"`
	Interval        types.Int64   `tfsdk:"interval"`
	RetryInterval   types.Int64   `tfsdk:"retry_interval"`
	ResendInterval  types.Int64   `tfsdk:"resend_interval"`
	MaxRetries      types.Int64   `tfsdk:"max_retries"`
	Timeout         types.Float64 `tfsdk:"timeout"`
	UpsideDown      types.Bool    `tfsdk:"upside_down"`
	Weight          types.Int64   `tfsdk:"weight"`
	ParentID        types.Int64   `tfsdk:"parent_id"`
	NotificationIDs types.Set     `tfsdk:"notification_ids"`
	Tags            types.Set     `tfsdk:"tags"`
}

// TagModel is one entry of the `tags` set.
type TagModel struct {
	TagID types.Int64  `tfsdk:"tag_id"`
	Value types.String `tfsdk:"value"`
}

// tagObjectType describes TagModel to the framework.
func tagObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"tag_id": types.Int64Type,
		"value":  types.StringType,
	}}
}

// Model is implemented by every per-type monitor model.
//
// The base resource drives the whole CRUD cycle and calls into these two hooks
// for whatever is specific to the type, so a new monitor type is a schema map
// plus two short methods.
type Model interface {
	// Base exposes the shared attributes.
	Base() *BaseModel
	// ApplyTo writes the type-specific attributes onto the wire payload.
	ApplyTo(ctx context.Context, wire *kuma.Monitor, diags *diag.Diagnostics)
	// ReadFrom reads the type-specific attributes back from the wire payload.
	ReadFrom(ctx context.Context, wire *kuma.Monitor, diags *diag.Diagnostics)
}

// TypeDef describes one monitor type resource.
type TypeDef struct {
	// TypeName is the resource name without the provider prefix, e.g.
	// "monitor_http".
	TypeName string
	// WireType is the value Uptime Kuma stores in monitor.type, e.g. "http".
	WireType string
	// Description is the resource-level documentation string.
	Description string
	// Attributes are the type-specific schema attributes, merged with the base.
	Attributes map[string]schema.Attribute
	// NewModel returns an empty model of the concrete type.
	NewModel func() Model
}

// Resource implements every monitor type. Behavior comes from the
// TypeDef it is built with.
type Resource struct {
	def    TypeDef
	client common.KumaClient
}

var (
	_ resource.Resource                = (*Resource)(nil)
	_ resource.ResourceWithConfigure   = (*Resource)(nil)
	_ resource.ResourceWithImportState = (*Resource)(nil)
)

// New returns a factory for the given monitor type.
func New(def TypeDef) func() resource.Resource {
	return func() resource.Resource {
		return &Resource{def: def}
	}
}

func (r *Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.def.TypeName
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
	attributes := baseAttributes()
	for name, attribute := range r.def.Attributes {
		if _, clash := attributes[name]; clash {
			// A type-specific attribute overriding a base one is a programming
			// error, and silently winning would be worse than panicking here.
			panic(fmt.Sprintf("monitor type %q redefines base attribute %q", r.def.TypeName, name))
		}
		attributes[name] = attribute
	}

	resp.Schema = schema.Schema{
		Description: r.def.Description,
		Attributes:  attributes,
	}
}

// baseAttributes builds the shared part of the schema.
//
// Attributes the server fills in are Optional+Computed: Uptime Kuma normalizes
// several values (a missing retry interval becomes the check interval, for
// instance), and marking them Computed lets the provider store what the server
// actually kept instead of fighting it every plan.
func baseAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Description: "Numeric ID of the monitor, assigned by Uptime Kuma.",
			Computed:    true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"name": schema.StringAttribute{
			Description: "Friendly name shown in the Uptime Kuma dashboard.",
			Required:    true,
		},
		"description": schema.StringAttribute{
			Description: "Description of the monitor.",
			Optional:    true,
		},
		"active": schema.BoolAttribute{
			Description: "Whether the monitor is running. Set to false to pause it. Default: true.",
			Optional:    true,
			Computed:    true,
		},
		"interval": schema.Int64Attribute{
			Description: "Seconds between checks. Default: 60.",
			Optional:    true,
			Computed:    true,
			Validators: []validator.Int64{
				int64validator.AtLeast(kuma.MinIntervalSeconds),
			},
		},
		"retry_interval": schema.Int64Attribute{
			Description: "Seconds between retries after a failure. Defaults to the value of `interval`, matching the Uptime Kuma web UI.",
			Optional:    true,
			Computed:    true,
			Validators: []validator.Int64{
				int64validator.AtLeast(kuma.MinIntervalSeconds),
			},
		},
		"resend_interval": schema.Int64Attribute{
			Description: "Resend the notification every N checks while the monitor stays down. 0 disables resending. Default: 0.",
			Optional:    true,
			Computed:    true,
			Validators: []validator.Int64{
				int64validator.AtLeast(0),
			},
		},
		"max_retries": schema.Int64Attribute{
			Description: "How many times to retry before marking the monitor down. Default: 0.",
			Optional:    true,
			Computed:    true,
			Validators: []validator.Int64{
				int64validator.AtLeast(0),
			},
		},
		"timeout": schema.Float64Attribute{
			Description: "Request timeout in seconds.",
			Optional:    true,
			Computed:    true,
		},
		"upside_down": schema.BoolAttribute{
			Description: "Invert the result: a reachable service counts as down. Default: false.",
			Optional:    true,
			Computed:    true,
		},
		"weight": schema.Int64Attribute{
			Description: "Sort weight in the dashboard; higher sorts first.",
			Optional:    true,
			Computed:    true,
		},
		"parent_id": schema.Int64Attribute{
			Description: "ID of the parent group monitor. Use with `uptimekuma_monitor_group`.",
			Optional:    true,
		},
		"notification_ids": schema.SetAttribute{
			Description: "IDs of the notification channels to trigger for this monitor.",
			Optional:    true,
			ElementType: types.Int64Type,
		},
		"tags": schema.SetNestedAttribute{
			Description: "Tags attached to this monitor. Uptime Kuma stores these as separate associations, so the provider reconciles them after saving the monitor itself.",
			Optional:    true,
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"tag_id": schema.Int64Attribute{
						Description: "ID of the tag, from `uptimekuma_tag`.",
						Required:    true,
					},
					"value": schema.StringAttribute{
						Description: "Optional per-monitor value for the tag, for example an environment name.",
						Optional:    true,
					},
				},
			},
		},
	}
}

// applyBase writes the shared attributes onto the wire payload.
func (m *BaseModel) applyBase(ctx context.Context, wire *kuma.Monitor, diags *diag.Diagnostics) {
	wire.Name = m.Name.ValueString()
	wire.Description = common.StringPtr(m.Description)

	wire.Interval = defaultInterval
	if common.IsSet(m.Interval) {
		wire.Interval = int(m.Interval.ValueInt64())
	}
	// Left at zero, the client mirrors the check interval, as the web UI does.
	if common.IsSet(m.RetryInterval) {
		wire.RetryInterval = int(m.RetryInterval.ValueInt64())
	}
	wire.ResendInterval = defaultResendInterval
	if common.IsSet(m.ResendInterval) {
		wire.ResendInterval = int(m.ResendInterval.ValueInt64())
	}
	wire.MaxRetries = defaultMaxRetries
	if common.IsSet(m.MaxRetries) {
		wire.MaxRetries = int(m.MaxRetries.ValueInt64())
	}

	wire.Timeout = common.Float64Ptr(m.Timeout)
	wire.Active = common.BoolPtr(m.Active)
	wire.UpsideDown = common.BoolPtr(m.UpsideDown)
	wire.Weight = common.IntPtr(m.Weight)
	wire.Parent = common.IntPtr(m.ParentID)

	// notificationIDList is a set encoded as an object keyed by stringified ID.
	// Always populated, empty map included: the server deletes and reinserts the
	// links from this map, so an empty one is how they get removed.
	ids := common.Int64SetToSlice(ctx, m.NotificationIDs)
	wire.NotificationIDList = make(map[string]bool, len(ids))
	for _, id := range ids {
		wire.NotificationIDList[strconv.Itoa(id)] = true
	}

	_ = diags
}

// readBase reads the shared attributes back from the wire payload.
func (m *BaseModel) readBase(wire *kuma.Monitor, diags *diag.Diagnostics) {
	m.ID = types.StringValue(strconv.Itoa(wire.ID))
	m.Name = types.StringValue(wire.Name)
	m.Description = common.StringValue(wire.Description)
	m.Interval = types.Int64Value(int64(wire.Interval))
	m.RetryInterval = types.Int64Value(int64(wire.RetryInterval))
	m.ResendInterval = types.Int64Value(int64(wire.ResendInterval))
	m.MaxRetries = types.Int64Value(int64(wire.MaxRetries))
	m.UpsideDown = common.BoolOrFalse(wire.UpsideDown)
	m.Active = common.BoolOrTrue(wire.Active)

	// The server always reports a weight, so mirroring it keeps the plan clean.
	m.Weight = common.IntValue(wire.Weight)
	m.ParentID = common.IntValue(wire.Parent)

	// Uptime Kuma returns 0 for "no timeout configured" on monitor types that
	// do not use one; leaving the attribute null in that case avoids showing a
	// value the user never set.
	if wire.Timeout != nil && *wire.Timeout > 0 {
		m.Timeout = types.Float64Value(*wire.Timeout)
	} else {
		m.Timeout = types.Float64Null()
	}

	if len(wire.NotificationIDList) > 0 {
		ids := make([]attr.Value, 0, len(wire.NotificationIDList))
		for key, enabled := range wire.NotificationIDList {
			if !enabled {
				continue
			}
			id, err := strconv.Atoi(key)
			if err != nil {
				continue
			}
			ids = append(ids, types.Int64Value(int64(id)))
		}
		set, setDiags := types.SetValue(types.Int64Type, ids)
		diags.Append(setDiags...)
		m.NotificationIDs = set
	} else {
		m.NotificationIDs = types.SetNull(types.Int64Type)
	}

	m.readTags(wire, diags)
}

// readTags mirrors the monitor's tag associations into the model.
func (m *BaseModel) readTags(wire *kuma.Monitor, diags *diag.Diagnostics) {
	if len(wire.Tags) == 0 {
		m.Tags = types.SetNull(tagObjectType())
		return
	}

	elements := make([]attr.Value, 0, len(wire.Tags))
	for _, tag := range wire.Tags {
		object, objectDiags := types.ObjectValue(
			tagObjectType().AttrTypes,
			map[string]attr.Value{
				"tag_id": types.Int64Value(int64(tag.TagID)),
				"value":  common.OptionalString(tag.Value),
			},
		)
		diags.Append(objectDiags...)
		elements = append(elements, object)
	}

	set, setDiags := types.SetValue(tagObjectType(), elements)
	diags.Append(setDiags...)
	m.Tags = set
}

// tagAssociations reads the desired tag set out of the model.
func (m *BaseModel) tagAssociations(ctx context.Context, diags *diag.Diagnostics) []TagModel {
	if !common.IsSet(m.Tags) {
		return nil
	}
	var tags []TagModel
	diags.Append(m.Tags.ElementsAs(ctx, &tags, false)...)
	return tags
}

func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	model := r.def.NewModel()
	resp.Diagnostics.Append(req.Plan.Get(ctx, model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	wire := &kuma.Monitor{Type: r.def.WireType}
	model.Base().applyBase(ctx, wire, &resp.Diagnostics)
	model.ApplyTo(ctx, wire, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	var id int
	err := common.RetryRPC(ctx, 3, func() error {
		var err error
		id, err = r.client.CreateMonitor(ctx, *wire)
		return err
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to create monitor", err.Error())
		return
	}

	tflog.Debug(ctx, "Created Uptime Kuma monitor", map[string]any{"id": id, "type": r.def.WireType})

	// Tags are separate associations, so they can only be attached once the
	// monitor exists.
	for _, tag := range model.Base().tagAssociations(ctx, &resp.Diagnostics) {
		if err := r.client.AddMonitorTag(ctx, int(tag.TagID.ValueInt64()), id, tag.Value.ValueString()); err != nil {
			resp.Diagnostics.AddError("Unable to attach tag to monitor", err.Error())
			return
		}
	}
	// `add` starts the monitor unless active was false, but it is cheap to
	// confirm and keeps Create and Update on the same path.
	r.reconcileActive(ctx, id, model.Base().Active, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Read back so every Computed attribute holds what the server decided.
	if !r.readInto(ctx, id, model, &resp.Diagnostics) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	model := r.def.NewModel()
	resp.Diagnostics.Append(req.State.Get(ctx, model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, ok := common.ParseID(model.Base().ID, &resp.Diagnostics)
	if !ok {
		return
	}

	wire, err := r.client.GetMonitor(ctx, id)
	if err != nil {
		if kuma.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read monitor", err.Error())
		return
	}

	// Guard against reading a monitor that another resource type owns, which
	// would otherwise silently rewrite unrelated state.
	if wire.Type != r.def.WireType {
		resp.Diagnostics.AddError(
			"Monitor type mismatch",
			fmt.Sprintf("Monitor %d is of type %q, but this resource manages %q monitors.", id, wire.Type, r.def.WireType),
		)
		return
	}

	model.Base().readBase(wire, &resp.Diagnostics)
	model.ReadFrom(ctx, wire, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	plan := r.def.NewModel()
	resp.Diagnostics.Append(req.Plan.Get(ctx, plan)...)
	state := r.def.NewModel()
	resp.Diagnostics.Append(req.State.Get(ctx, state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, ok := common.ParseID(state.Base().ID, &resp.Diagnostics)
	if !ok {
		return
	}

	// The payload is built from the plan alone, not merged onto what the server
	// currently holds.
	//
	// editMonitor writes every field it is given, so merging would look safer —
	// but it makes removing an attribute impossible: a value the user deleted
	// from the config arrives as null, leaves the wire struct untouched, and the
	// server's old value gets written straight back. Building from the plan makes
	// the config authoritative, which is what Terraform users expect. The
	// trade-off is that fields this provider does not model (such as the
	// condition tree editable in the web UI) are reset on update.
	wire := &kuma.Monitor{ID: id, Type: r.def.WireType}
	plan.Base().applyBase(ctx, wire, &resp.Diagnostics)
	plan.ApplyTo(ctx, wire, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	err := common.RetryRPC(ctx, 3, func() error {
		return r.client.UpdateMonitor(ctx, *wire)
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to update monitor", err.Error())
		return
	}

	r.reconcileTags(ctx, id, state.Base(), plan.Base(), &resp.Diagnostics)
	r.reconcileActive(ctx, id, plan.Base().Active, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	if !r.readInto(ctx, id, plan, &resp.Diagnostics) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// reconcileActive pauses or resumes the monitor to match the plan.
//
// editMonitor deliberately never writes the `active` column — it assigns every
// other field one by one and leaves that one alone — so sending active in the
// payload has no effect on an existing monitor. Pausing and resuming are their
// own events, and this is the only thing that actually moves the state.
func (r *Resource) reconcileActive(ctx context.Context, id int, desired types.Bool, diags *diag.Diagnostics) {
	if !common.IsSet(desired) {
		return
	}

	wire, err := r.client.GetMonitor(ctx, id)
	if err != nil {
		diags.AddError("Unable to read monitor state before pausing or resuming", err.Error())
		return
	}

	current := true
	if wire.Active != nil {
		current = bool(*wire.Active)
	}
	if current == desired.ValueBool() {
		return
	}

	if desired.ValueBool() {
		err = r.client.ResumeMonitor(ctx, id)
	} else {
		err = r.client.PauseMonitor(ctx, id)
	}
	if err != nil {
		diags.AddError("Unable to change the monitor's active state", err.Error())
	}
}

func (r *Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	model := r.def.NewModel()
	resp.Diagnostics.Append(req.State.Get(ctx, model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, ok := common.ParseID(model.Base().ID, &resp.Diagnostics)
	if !ok {
		return
	}

	// deleteChildren is false: children of a group are managed by their own
	// resources, and cascading would delete state Terraform still tracks.
	err := common.RetryRPC(ctx, 3, func() error {
		return r.client.DeleteMonitor(ctx, id, false)
	})
	if err != nil && !kuma.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete monitor", err.Error())
	}
}

func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if _, err := strconv.Atoi(req.ID); err != nil {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected the numeric monitor ID, got %q.", req.ID),
		)
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// readInto refreshes a model from the server.
func (r *Resource) readInto(ctx context.Context, id int, model Model, diags *diag.Diagnostics) bool {
	wire, err := r.client.GetMonitor(ctx, id)
	if err != nil {
		diags.AddError("Unable to read monitor back after saving", err.Error())
		return false
	}
	model.Base().readBase(wire, diags)
	model.ReadFrom(ctx, wire, diags)
	return !diags.HasError()
}

// reconcileTags brings the monitor's tag associations in line with the plan.
//
// Tag associations are their own events, and the delete event identifies an
// association by (tag, monitor, value) — so a changed value is a remove plus an
// add, not an update.
func (r *Resource) reconcileTags(ctx context.Context, monitorID int, state, plan *BaseModel, diags *diag.Diagnostics) {
	current := state.tagAssociations(ctx, diags)
	desired := plan.tagAssociations(ctx, diags)
	if diags.HasError() {
		return
	}

	type key struct {
		tagID int
		value string
	}
	currentSet := make(map[key]struct{}, len(current))
	for _, tag := range current {
		currentSet[key{int(tag.TagID.ValueInt64()), tag.Value.ValueString()}] = struct{}{}
	}
	desiredSet := make(map[key]struct{}, len(desired))
	for _, tag := range desired {
		desiredSet[key{int(tag.TagID.ValueInt64()), tag.Value.ValueString()}] = struct{}{}
	}

	for k := range currentSet {
		if _, keep := desiredSet[k]; keep {
			continue
		}
		if err := r.client.DeleteMonitorTag(ctx, k.tagID, monitorID, k.value); err != nil {
			diags.AddError("Unable to detach tag from monitor", err.Error())
			return
		}
	}
	for k := range desiredSet {
		if _, exists := currentSet[k]; exists {
			continue
		}
		if err := r.client.AddMonitorTag(ctx, k.tagID, monitorID, k.value); err != nil {
			diags.AddError("Unable to attach tag to monitor", err.Error())
			return
		}
	}
}
