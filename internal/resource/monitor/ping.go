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

// PingModel is an ICMP ping monitor.
type PingModel struct {
	BaseModel

	Hostname              types.String `tfsdk:"hostname"`
	PacketSize            types.Int64  `tfsdk:"packet_size"`
	PingCount             types.Int64  `tfsdk:"ping_count"`
	PingPerRequestTimeout types.Int64  `tfsdk:"ping_per_request_timeout"`
	PingNumeric           types.Bool   `tfsdk:"ping_numeric"`
	IPFamily              types.String `tfsdk:"ip_family"`
}

var _ Model = (*PingModel)(nil)

func (m *PingModel) Base() *BaseModel { return &m.BaseModel }

func (m *PingModel) ApplyTo(ctx context.Context, wire *kuma.Monitor, diags *diag.Diagnostics) {
	wire.Hostname = common.StringPtr(m.Hostname)
	wire.PacketSize = common.IntPtr(m.PacketSize)
	wire.PingCount = common.IntPtr(m.PingCount)
	wire.PingPerRequestTimeout = common.IntPtr(m.PingPerRequestTimeout)
	wire.PingNumeric = common.BoolPtr(m.PingNumeric)
	wire.IPFamily = common.StringPtr(m.IPFamily)
	_ = ctx
	_ = diags
}

func (m *PingModel) ReadFrom(ctx context.Context, wire *kuma.Monitor, diags *diag.Diagnostics) {
	m.Hostname = common.StringValue(wire.Hostname)
	m.PacketSize = common.IntValue(wire.PacketSize)
	m.PingCount = common.IntValue(wire.PingCount)
	m.PingPerRequestTimeout = common.IntValue(wire.PingPerRequestTimeout)
	m.PingNumeric = common.BoolOrFalse(wire.PingNumeric)
	m.IPFamily = common.OptionalString(wire.IPFamily)
	_ = ctx
	_ = diags
}

// NewPingResource returns the uptimekuma_monitor_ping resource.
func NewPingResource() resource.Resource {
	return New(TypeDef{
		TypeName:    "monitor_ping",
		WireType:    "ping",
		Description: "Monitors a host with ICMP echo requests.",
		Attributes: map[string]schema.Attribute{
			"hostname": schema.StringAttribute{
				Description: "Hostname or IP address to ping.",
				Required:    true,
			},
			"packet_size": schema.Int64Attribute{
				Description: "ICMP payload size in bytes. Default: 56.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.Int64{
					int64validator.Between(1, 65500),
				},
			},
			"ping_count": schema.Int64Attribute{
				Description: "How many echo requests to send per check.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"ping_per_request_timeout": schema.Int64Attribute{
				Description: "Timeout for each individual echo request, in seconds. Must not exceed `timeout`.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"ping_numeric": schema.BoolAttribute{
				Description: "Skip reverse DNS lookups on the response. Default: false.",
				Optional:    true,
				Computed:    true,
			},
			"ip_family": schema.StringAttribute{
				Description: "Force an address family: `ipv4` or `ipv6`. Leave unset to let the resolver choose.",
				Optional:    true,
				Validators: []validator.String{
					stringvalidator.OneOf("ipv4", "ipv6"),
				},
			},
		},
		NewModel: func() Model { return &PingModel{} },
	})()
}
