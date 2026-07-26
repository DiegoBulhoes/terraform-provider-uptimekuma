package monitor

import (
	"context"
	"sort"
	"strconv"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ListDataSource lists monitors, optionally filtered by type.
type ListDataSource struct {
	client common.KumaClient
}

type ListModel struct {
	Type     types.String `tfsdk:"type"`
	Monitors []ListEntry  `tfsdk:"monitors"`
	IDs      types.List   `tfsdk:"ids"`
}

// ListEntry is one monitor in the list.
type ListEntry struct {
	ID       types.String `tfsdk:"id"`
	Name     types.String `tfsdk:"name"`
	Type     types.String `tfsdk:"type"`
	URL      types.String `tfsdk:"url"`
	Hostname types.String `tfsdk:"hostname"`
	Active   types.Bool   `tfsdk:"active"`
	Interval types.Int64  `tfsdk:"interval"`
	ParentID types.Int64  `tfsdk:"parent_id"`
}

var (
	_ datasource.DataSource              = (*ListDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*ListDataSource)(nil)
)

func NewList() datasource.DataSource {
	return &ListDataSource{}
}

func (d *ListDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_monitors"
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
		Description: "Lists every monitor, optionally narrowed to one type.",
		Attributes: map[string]schema.Attribute{
			"type": schema.StringAttribute{
				Description: "Only return monitors of this type, for example `http`. Omit to return all of them.",
				Optional:    true,
			},
			"ids": schema.ListAttribute{
				Description: "IDs of the matching monitors, in the same order as `monitors`.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"monitors": schema.ListNestedAttribute{
				Description: "The matching monitors, ordered by ID.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":        schema.StringAttribute{Computed: true, Description: "Numeric ID of the monitor."},
						"name":      schema.StringAttribute{Computed: true, Description: "Name of the monitor."},
						"type":      schema.StringAttribute{Computed: true, Description: "Monitor type."},
						"url":       schema.StringAttribute{Computed: true, Description: "Target URL, for HTTP-based types."},
						"hostname":  schema.StringAttribute{Computed: true, Description: "Target hostname, for host-based types."},
						"active":    schema.BoolAttribute{Computed: true, Description: "Whether the monitor is running."},
						"interval":  schema.Int64Attribute{Computed: true, Description: "Seconds between checks."},
						"parent_id": schema.Int64Attribute{Computed: true, Description: "ID of the parent group monitor."},
					},
				},
			},
		},
	}
}

func (d *ListDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var model ListModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var monitors map[int]kuma.Monitor
	err := common.RetryRPC(ctx, 3, func() error {
		var err error
		monitors, err = d.client.ListMonitors(ctx)
		return err
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to list monitors", err.Error())
		return
	}

	// The API returns an unordered map, so sort by ID to keep the output stable
	// between plans.
	ids := make([]int, 0, len(monitors))
	for id := range monitors {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	filterType := model.Type.ValueString()
	entries := make([]ListEntry, 0, len(ids))
	idValues := make([]attr.Value, 0, len(ids))

	for _, id := range ids {
		monitor := monitors[id]
		if common.IsSet(model.Type) && monitor.Type != filterType {
			continue
		}
		entries = append(entries, ListEntry{
			ID:       types.StringValue(strconv.Itoa(monitor.ID)),
			Name:     types.StringValue(monitor.Name),
			Type:     types.StringValue(monitor.Type),
			URL:      common.StringValue(monitor.URL),
			Hostname: common.StringValue(monitor.Hostname),
			Active:   types.BoolValue(monitor.Active.Value()),
			Interval: types.Int64Value(int64(monitor.Interval)),
			ParentID: common.IntValue(monitor.Parent),
		})
		idValues = append(idValues, types.StringValue(strconv.Itoa(monitor.ID)))
	}

	idList, listDiags := types.ListValue(types.StringType, idValues)
	resp.Diagnostics.Append(listDiags...)

	model.Monitors = entries
	model.IDs = idList
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}
