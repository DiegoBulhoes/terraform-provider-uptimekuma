// Package monitor exposes monitors for reading: one by id or name, and the
// full list with an optional type filter.
package monitor

import (
	"context"
	"fmt"
	"strconv"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// DataSource looks up a single monitor by ID or by name.
type DataSource struct {
	client common.KumaClient
}

type Model struct {
	ID          types.String `tfsdk:"id"`
	Name        types.String `tfsdk:"name"`
	Type        types.String `tfsdk:"type"`
	Description types.String `tfsdk:"description"`
	URL         types.String `tfsdk:"url"`
	Hostname    types.String `tfsdk:"hostname"`
	Port        types.Int64  `tfsdk:"port"`
	Active      types.Bool   `tfsdk:"active"`
	Interval    types.Int64  `tfsdk:"interval"`
	ParentID    types.Int64  `tfsdk:"parent_id"`
	Tags        types.Set    `tfsdk:"tags"`
}

var (
	_ datasource.DataSource              = (*DataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*DataSource)(nil)
)

func New() datasource.DataSource {
	return &DataSource{}
}

func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_monitor"
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
		Description: "Looks up a monitor by ID or by name. Exposes the attributes common to every monitor type; " +
			"for the type-specific ones, read the monitor's own resource.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Numeric ID of the monitor. Provide either this or `name`.",
				Optional:    true,
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "Name of the monitor. Provide either this or `id`. Must match exactly one monitor.",
				Optional:    true,
				Computed:    true,
			},
			"type": schema.StringAttribute{
				Description: "Monitor type, for example `http` or `ping`.",
				Computed:    true,
			},
			"description": schema.StringAttribute{
				Description: "Description of the monitor.",
				Computed:    true,
			},
			"url": schema.StringAttribute{
				Description: "Target URL, for the HTTP-based types.",
				Computed:    true,
			},
			"hostname": schema.StringAttribute{
				Description: "Target hostname, for the host-based types.",
				Computed:    true,
			},
			"port": schema.Int64Attribute{
				Description: "Target port, where the type uses one.",
				Computed:    true,
			},
			"active": schema.BoolAttribute{
				Description: "Whether the monitor is currently running.",
				Computed:    true,
			},
			"interval": schema.Int64Attribute{
				Description: "Seconds between checks.",
				Computed:    true,
			},
			"parent_id": schema.Int64Attribute{
				Description: "ID of the parent group monitor, if any.",
				Computed:    true,
			},
			"tags": schema.SetNestedAttribute{
				Description: "Tags attached to the monitor.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"tag_id": schema.Int64Attribute{Computed: true, Description: "ID of the tag."},
						"name":   schema.StringAttribute{Computed: true, Description: "Name of the tag."},
						"color":  schema.StringAttribute{Computed: true, Description: "Color of the tag."},
						"value":  schema.StringAttribute{Computed: true, Description: "Per-monitor value of the tag."},
					},
				},
			},
		},
	}
}

func (d *DataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var model Model
	resp.Diagnostics.Append(req.Config.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	hasID := common.IsSet(model.ID)
	hasName := common.IsSet(model.Name)
	switch {
	case hasID && hasName:
		resp.Diagnostics.AddError("Ambiguous monitor lookup", "Set either `id` or `name`, not both.")
		return
	case !hasID && !hasName:
		resp.Diagnostics.AddError("Missing monitor lookup", "Set either `id` or `name`.")
		return
	}

	var monitor *kuma.Monitor
	if hasID {
		id, ok := common.ParseID(model.ID, &resp.Diagnostics)
		if !ok {
			return
		}
		err := common.RetryRPC(ctx, 3, func() error {
			var err error
			monitor, err = d.client.GetMonitor(ctx, id)
			return err
		})
		if err != nil {
			resp.Diagnostics.AddError("Unable to read monitor", err.Error())
			return
		}
	} else {
		found, err := findByName(ctx, d.client, model.Name.ValueString())
		if err != nil {
			resp.Diagnostics.AddError("Unable to find monitor", err.Error())
			return
		}
		monitor = found
	}

	model.ID = types.StringValue(strconv.Itoa(monitor.ID))
	model.Name = types.StringValue(monitor.Name)
	model.Type = types.StringValue(monitor.Type)
	model.Description = common.StringValue(monitor.Description)
	model.URL = common.StringValue(monitor.URL)
	model.Hostname = common.StringValue(monitor.Hostname)
	model.Port = common.IntValue(monitor.Port)
	model.Active = types.BoolValue(monitor.Active.Value())
	model.Interval = types.Int64Value(int64(monitor.Interval))
	model.ParentID = common.IntValue(monitor.Parent)
	model.Tags = tagSet(monitor.Tags, &resp.Diagnostics)

	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// findByName resolves a monitor name against the pushed list, requiring
// exactly one match so a typo cannot silently pick the wrong monitor.
func findByName(ctx context.Context, client common.KumaClient, name string) (*kuma.Monitor, error) {
	var monitors map[int]kuma.Monitor
	err := common.RetryRPC(ctx, 3, func() error {
		var err error
		monitors, err = client.ListMonitors(ctx)
		return err
	})
	if err != nil {
		return nil, err
	}

	var matches []kuma.Monitor
	for _, monitor := range monitors {
		if monitor.Name == name {
			matches = append(matches, monitor)
		}
	}

	switch len(matches) {
	case 0:
		return nil, fmt.Errorf("no monitor named %q: %w", name, kuma.ErrNotFound)
	case 1:
		return &matches[0], nil
	default:
		return nil, fmt.Errorf("%d monitors are named %q; look them up by id instead", len(matches), name)
	}
}
