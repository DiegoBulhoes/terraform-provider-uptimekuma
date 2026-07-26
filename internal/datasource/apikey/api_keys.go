// Package apikey exposes the API keys for reading, without their secrets.
package apikey

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
	APIKeys []ListEntry `tfsdk:"api_keys"`
}

type ListEntry struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Active  types.Bool   `tfsdk:"active"`
	Expires types.String `tfsdk:"expires"`
	Status  types.String `tfsdk:"status"`
}

var (
	_ datasource.DataSource              = (*ListDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*ListDataSource)(nil)
)

func NewList() datasource.DataSource {
	return &ListDataSource{}
}

func (d *ListDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_api_keys"
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
		Description: "Lists the API keys. The secrets themselves are never returned — Uptime Kuma only stores hashes.",
		Attributes: map[string]schema.Attribute{
			"api_keys": schema.ListNestedAttribute{
				Description: "The API keys, ordered by ID.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":      schema.StringAttribute{Computed: true, Description: "Numeric ID of the key."},
						"name":    schema.StringAttribute{Computed: true, Description: "Name of the key."},
						"active":  schema.BoolAttribute{Computed: true, Description: "Whether the key is enabled."},
						"expires": schema.StringAttribute{Computed: true, Description: "Expiry timestamp, if set."},
						"status":  schema.StringAttribute{Computed: true, Description: "`active`, `inactive` or `expired`."},
					},
				},
			},
		},
	}
}

func (d *ListDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	keys, err := d.client.ListAPIKeys(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list API keys", err.Error())
		return
	}

	ids := make([]int, 0, len(keys))
	for id := range keys {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	entries := make([]ListEntry, 0, len(ids))
	for _, id := range ids {
		key := keys[id]
		entries = append(entries, ListEntry{
			ID:      types.StringValue(strconv.Itoa(key.ID)),
			Name:    types.StringValue(key.Name),
			Active:  types.BoolValue(bool(key.Active)),
			Expires: common.OptionalString(key.Expires),
			Status:  types.StringValue(key.Status),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &ListModel{APIKeys: entries})...)
}
