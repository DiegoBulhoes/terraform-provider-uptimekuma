package monitor

import (
	"context"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// PortModel is a TCP port monitor.
type PortModel struct {
	BaseModel

	Hostname types.String `tfsdk:"hostname"`
	Port     types.Int64  `tfsdk:"port"`
	IPFamily types.String `tfsdk:"ip_family"`
}

var _ Model = (*PortModel)(nil)

func (m *PortModel) Base() *BaseModel { return &m.BaseModel }

func (m *PortModel) ApplyTo(ctx context.Context, wire *kuma.Monitor, diags *diag.Diagnostics) {
	wire.Hostname = common.StringPtr(m.Hostname)
	wire.Port = common.IntPtr(m.Port)
	wire.IPFamily = common.StringPtr(m.IPFamily)
	_ = ctx
	_ = diags
}

func (m *PortModel) ReadFrom(ctx context.Context, wire *kuma.Monitor, diags *diag.Diagnostics) {
	m.Hostname = common.StringValue(wire.Hostname)
	m.Port = common.IntValue(wire.Port)
	m.IPFamily = common.OptionalString(wire.IPFamily)
	_ = ctx
	_ = diags
}

// NewPortResource returns the uptimekuma_monitor_port resource.
func NewPortResource() resource.Resource {
	return New(TypeDef{
		TypeName:    "monitor_port",
		WireType:    "port",
		Description: "Monitors a TCP port by opening a connection to it.",
		Attributes: map[string]schema.Attribute{
			"hostname": schema.StringAttribute{
				Description: "Hostname or IP address to connect to.",
				Required:    true,
			},
			"port": schema.Int64Attribute{
				Description: "TCP port to connect to.",
				Required:    true,
				Validators: []validator.Int64{
					int64validator.Between(1, 65535),
				},
			},
			"ip_family": schema.StringAttribute{
				Description: "Force an address family: `ipv4` or `ipv6`. Leave unset to let the resolver choose.",
				Optional:    true,
				Validators: []validator.String{
					stringvalidator.OneOf("ipv4", "ipv6"),
				},
			},
		},
		NewModel: func() Model { return &PortModel{} },
	})()
}
