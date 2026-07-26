// Package maintenance exposes the maintenance windows for reading.
package maintenance

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

// ListDataSource lists the maintenance windows.
type ListDataSource struct {
	client common.KumaClient
}

type ListModel struct {
	Maintenances []ListEntry `tfsdk:"maintenances"`
}

type ListEntry struct {
	ID          types.String `tfsdk:"id"`
	Title       types.String `tfsdk:"title"`
	Description types.String `tfsdk:"description"`
	Strategy    types.String `tfsdk:"strategy"`
	Active      types.Bool   `tfsdk:"active"`
	Status      types.String `tfsdk:"status"`
}

var (
	_ datasource.DataSource              = (*ListDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*ListDataSource)(nil)
)

func NewList() datasource.DataSource {
	return &ListDataSource{}
}

func (d *ListDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_maintenances"
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
		Description: "Lists the maintenance windows on the Uptime Kuma instance.",
		Attributes: map[string]schema.Attribute{
			"maintenances": schema.ListNestedAttribute{
				Description: "The maintenance windows, ordered by ID.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":          schema.StringAttribute{Computed: true, Description: "Numeric ID of the window."},
						"title":       schema.StringAttribute{Computed: true, Description: "Title of the window."},
						"description": schema.StringAttribute{Computed: true, Description: "Description of the maintenance."},
						"strategy":    schema.StringAttribute{Computed: true, Description: "Scheduling strategy."},
						"active":      schema.BoolAttribute{Computed: true, Description: "Whether the window is active."},
						"status":      schema.StringAttribute{Computed: true, Description: "Server-computed status."},
					},
				},
			},
		},
	}
}

func (d *ListDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var maintenances map[int]kuma.Maintenance
	err := common.RetryRPC(ctx, 3, func() error {
		var err error
		maintenances, err = d.client.ListMaintenances(ctx)
		return err
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to list maintenance windows", err.Error())
		return
	}

	ids := make([]int, 0, len(maintenances))
	for id := range maintenances {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	entries := make([]ListEntry, 0, len(ids))
	for _, id := range ids {
		maintenance := maintenances[id]
		entries = append(entries, ListEntry{
			ID:          types.StringValue(strconv.Itoa(maintenance.ID)),
			Title:       types.StringValue(maintenance.Title),
			Description: types.StringValue(maintenance.Description),
			Strategy:    types.StringValue(maintenance.Strategy),
			Active:      types.BoolValue(maintenance.Active.Value()),
			Status:      types.StringValue(maintenance.Status),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &ListModel{Maintenances: entries})...)
}
