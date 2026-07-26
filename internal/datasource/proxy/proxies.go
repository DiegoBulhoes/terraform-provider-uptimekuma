// Package proxy exposes the configured proxies for reading.
package proxy

import (
	"context"
	"sort"
	"strconv"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type ListDataSource struct {
	client common.KumaClient
}

type ListModel struct {
	Proxies []ListEntry `tfsdk:"proxies"`
}

type ListEntry struct {
	ID       types.String `tfsdk:"id"`
	Protocol types.String `tfsdk:"protocol"`
	Host     types.String `tfsdk:"host"`
	Port     types.Int64  `tfsdk:"port"`
	Active   types.Bool   `tfsdk:"active"`
	Default  types.Bool   `tfsdk:"default"`
}

var (
	_ datasource.DataSource              = (*ListDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*ListDataSource)(nil)
)

func NewList() datasource.DataSource {
	return &ListDataSource{}
}

func (d *ListDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_proxies"
}

func (d *ListDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, err := common.ConfigureClient(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected provider data", err.Error())
		return
	}
	d.client = client
}

func (d *ListDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists the proxies configured on the Uptime Kuma instance. Credentials are not exposed.",
		Attributes: map[string]schema.Attribute{
			"proxies": schema.ListNestedAttribute{
				Description: "The proxies, ordered by ID.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":       schema.StringAttribute{Computed: true, Description: "Numeric ID of the proxy."},
						"protocol": schema.StringAttribute{Computed: true, Description: "Proxy protocol."},
						"host":     schema.StringAttribute{Computed: true, Description: "Proxy host."},
						"port":     schema.Int64Attribute{Computed: true, Description: "Proxy port."},
						"active":   schema.BoolAttribute{Computed: true, Description: "Whether the proxy is usable."},
						"default":  schema.BoolAttribute{Computed: true, Description: "Whether it is the default for new monitors."},
					},
				},
			},
		},
	}
}

func (d *ListDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var proxies map[int]kuma.Proxy
	err := common.RetryRPC(ctx, 3, func() error {
		var err error
		proxies, err = d.client.ListProxies(ctx)
		return err
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to list proxies", err.Error())
		return
	}

	ids := make([]int, 0, len(proxies))
	for id := range proxies {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	entries := make([]ListEntry, 0, len(ids))
	for _, id := range ids {
		proxy := proxies[id]
		entries = append(entries, ListEntry{
			ID:       types.StringValue(strconv.Itoa(proxy.ID)),
			Protocol: types.StringValue(proxy.Protocol),
			Host:     types.StringValue(proxy.Host),
			Port:     types.Int64Value(int64(proxy.Port)),
			Active:   types.BoolValue(bool(proxy.Active)),
			Default:  types.BoolValue(bool(proxy.Default)),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &ListModel{Proxies: entries})...)
}
