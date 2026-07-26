// Package dockerhost exposes the configured Docker hosts for reading.
package dockerhost

import (
	"context"
	"sort"
	"strconv"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

type ListDataSource struct {
	client common.KumaClient
}

type ListModel struct {
	DockerHosts []ListEntry `tfsdk:"docker_hosts"`
}

type ListEntry struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	ConnectionType types.String `tfsdk:"connection_type"`
	Daemon         types.String `tfsdk:"daemon"`
}

var (
	_ datasource.DataSource              = (*ListDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*ListDataSource)(nil)
)

func NewList() datasource.DataSource {
	return &ListDataSource{}
}

func (d *ListDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_docker_hosts"
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
		Description: "Lists the Docker hosts configured on the Uptime Kuma instance.",
		Attributes: map[string]schema.Attribute{
			"docker_hosts": schema.ListNestedAttribute{
				Description: "The Docker hosts, ordered by ID.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":              schema.StringAttribute{Computed: true, Description: "Numeric ID of the Docker host."},
						"name":            schema.StringAttribute{Computed: true, Description: "Name of the Docker host."},
						"connection_type": schema.StringAttribute{Computed: true, Description: "`socket` or `tcp`."},
						"daemon":          schema.StringAttribute{Computed: true, Description: "Socket path or TCP address."},
					},
				},
			},
		},
	}
}

func (d *ListDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	hosts, err := d.client.ListDockerHosts(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list Docker hosts", err.Error())
		return
	}

	ids := make([]int, 0, len(hosts))
	for id := range hosts {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	entries := make([]ListEntry, 0, len(ids))
	for _, id := range ids {
		host := hosts[id]
		entries = append(entries, ListEntry{
			ID:             types.StringValue(strconv.Itoa(host.ID)),
			Name:           types.StringValue(host.Name),
			ConnectionType: types.StringValue(host.DockerType),
			Daemon:         types.StringValue(host.DockerDaemon),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &ListModel{DockerHosts: entries})...)
}
