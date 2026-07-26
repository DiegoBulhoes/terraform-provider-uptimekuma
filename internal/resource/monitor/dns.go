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

// DNSModel is a DNS resolution monitor.
type DNSModel struct {
	BaseModel

	Hostname         types.String `tfsdk:"hostname"`
	DNSResolveServer types.String `tfsdk:"resolver_server"`
	Port             types.Int64  `tfsdk:"port"`
	DNSResolveType   types.String `tfsdk:"resolve_type"`
	LastResult       types.String `tfsdk:"last_result"`
}

var _ Model = (*DNSModel)(nil)

func (m *DNSModel) Base() *BaseModel { return &m.BaseModel }

func (m *DNSModel) ApplyTo(ctx context.Context, wire *kuma.Monitor, diags *diag.Diagnostics) {
	wire.Hostname = common.StringPtr(m.Hostname)
	wire.DNSResolveServer = common.StringPtr(m.DNSResolveServer)
	wire.Port = common.IntPtr(m.Port)
	wire.DNSResolveType = common.StringPtr(m.DNSResolveType)
	_ = ctx
	_ = diags
}

func (m *DNSModel) ReadFrom(ctx context.Context, wire *kuma.Monitor, diags *diag.Diagnostics) {
	m.Hostname = common.StringValue(wire.Hostname)
	m.DNSResolveServer = common.StringValue(wire.DNSResolveServer)
	m.Port = common.IntValue(wire.Port)
	m.DNSResolveType = common.StringValue(wire.DNSResolveType)
	// Populated by the server from the most recent check.
	m.LastResult = common.OptionalString(wire.DNSLastResult)
	_ = ctx
	_ = diags
}

// NewDNSResource returns the uptimekuma_monitor_dns resource.
func NewDNSResource() resource.Resource {
	return New(TypeDef{
		TypeName:    "monitor_dns",
		WireType:    "dns",
		Description: "Monitors a DNS record by querying a resolver for it.",
		Attributes: map[string]schema.Attribute{
			"hostname": schema.StringAttribute{
				Description: "Hostname to resolve.",
				Required:    true,
			},
			"resolver_server": schema.StringAttribute{
				Description: "IP address of the resolver to query. Default: 1.1.1.1.",
				Optional:    true,
				Computed:    true,
			},
			"port": schema.Int64Attribute{
				Description: "Port of the resolver. Default: 53.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.Int64{
					int64validator.Between(1, 65535),
				},
			},
			"resolve_type": schema.StringAttribute{
				Description: "Record type to query. Default: A.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.String{
					stringvalidator.OneOf("A", "AAAA", "CAA", "CNAME", "MX", "NS", "PTR", "SOA", "SRV", "TXT"),
				},
			},
			"last_result": schema.StringAttribute{
				Description: "Result of the most recent resolution, as reported by the server.",
				Computed:    true,
			},
		},
		NewModel: func() Model { return &DNSModel{} },
	})()
}
