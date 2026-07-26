// Package info exposes the server metadata Uptime Kuma reports after login.
package info

import (
	"context"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// DataSource exposes the server metadata Uptime Kuma pushes after login.
//
// Useful for gating configuration on the server version, and for the
// `is_container` flag: a containerised instance cannot run the sip-options,
// tailscale-ping or system-service monitor types.
type DataSource struct {
	client common.KumaClient
}

type Model struct {
	Version        types.String `tfsdk:"version"`
	LatestVersion  types.String `tfsdk:"latest_version"`
	PrimaryBaseURL types.String `tfsdk:"primary_base_url"`
	ServerTimezone types.String `tfsdk:"server_timezone"`
	IsContainer    types.Bool   `tfsdk:"is_container"`
	DatabaseType   types.String `tfsdk:"database_type"`
}

var (
	_ datasource.DataSource              = (*DataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*DataSource)(nil)
)

func New() datasource.DataSource {
	return &DataSource{}
}

func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_info"
}

func (d *DataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Metadata about the Uptime Kuma server, as reported after login.",
		Attributes: map[string]schema.Attribute{
			"version":          schema.StringAttribute{Computed: true, Description: "Running Uptime Kuma version."},
			"latest_version":   schema.StringAttribute{Computed: true, Description: "Latest version the server knows about."},
			"primary_base_url": schema.StringAttribute{Computed: true, Description: "Configured primary base URL, if any."},
			"server_timezone":  schema.StringAttribute{Computed: true, Description: "Server timezone."},
			"is_container": schema.BoolAttribute{
				Computed: true,
				Description: "Whether the server runs in a container. Containerised instances cannot use the " +
					"sip-options, tailscale-ping or system-service monitor types.",
			},
			"database_type": schema.StringAttribute{Computed: true, Description: "Database backend, for example `sqlite` or `mariadb`."},
		},
	}
}

func (d *DataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	info := d.client.Info()

	resp.Diagnostics.Append(resp.State.Set(ctx, &Model{
		Version:        types.StringValue(info.Version),
		LatestVersion:  types.StringValue(info.LatestVersion),
		PrimaryBaseURL: types.StringValue(info.PrimaryBaseURL),
		ServerTimezone: types.StringValue(info.ServerTimezone),
		IsContainer:    types.BoolValue(bool(info.IsContainer)),
		DatabaseType:   types.StringValue(info.DBType),
	})...)
}
