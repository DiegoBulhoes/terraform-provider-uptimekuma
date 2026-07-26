package monitor

import (
	"context"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// GroupModel is a container monitor. It runs no check of its own; other
// monitors point at it through parent_id and it aggregates their status.
type GroupModel struct {
	BaseModel

	ChildrenIDs types.Set `tfsdk:"children_ids"`
}

var _ Model = (*GroupModel)(nil)

func (m *GroupModel) Base() *BaseModel { return &m.BaseModel }

func (m *GroupModel) ApplyTo(_ context.Context, _ *kuma.Monitor, _ *diag.Diagnostics) {
	// A group has no settings of its own; membership lives on the children.
}

func (m *GroupModel) ReadFrom(ctx context.Context, wire *kuma.Monitor, diags *diag.Diagnostics) {
	if len(wire.ChildrenIDs) == 0 {
		m.ChildrenIDs = types.SetNull(types.Int64Type)
		return
	}
	elements := make([]attr.Value, 0, len(wire.ChildrenIDs))
	for _, id := range wire.ChildrenIDs {
		elements = append(elements, types.Int64Value(int64(id)))
	}
	set, setDiags := types.SetValue(types.Int64Type, elements)
	diags.Append(setDiags...)
	m.ChildrenIDs = set
	_ = ctx
}

// NewGroupResource returns the uptimekuma_monitor_group resource.
func NewGroupResource() resource.Resource {
	return New(TypeDef{
		TypeName: "monitor_group",
		WireType: "group",
		Description: "A group monitor. It performs no check itself; point other monitors at it with " +
			"`parent_id` and it reports their aggregate status.",
		Attributes: map[string]schema.Attribute{
			"children_ids": schema.SetAttribute{
				Description: "IDs of the monitors currently inside this group. Membership is set on the child, through its `parent_id`.",
				Computed:    true,
				ElementType: types.Int64Type,
			},
		},
		NewModel: func() Model { return &GroupModel{} },
	})()
}
