package monitor

import (
	"context"
	"strconv"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The models every monitor type embeds, and the descriptor a type is built from.

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

// Conversions for the attributes every monitor type shares.

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
