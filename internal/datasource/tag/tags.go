package tag

import (
	"context"
	"sort"
	"strconv"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ListDataSource lists every tag.
type ListDataSource struct {
	client common.KumaClient
}

type ListModel struct {
	Tags []ListEntry `tfsdk:"tags"`
}

type ListEntry struct {
	ID    types.String `tfsdk:"id"`
	Name  types.String `tfsdk:"name"`
	Color types.String `tfsdk:"color"`
}

var (
	_ datasource.DataSource              = (*ListDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*ListDataSource)(nil)
)

func NewList() datasource.DataSource {
	return &ListDataSource{}
}

func (d *ListDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tags"
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
		Description: "Lists every tag defined on the Uptime Kuma instance.",
		Attributes: map[string]schema.Attribute{
			"tags": schema.ListNestedAttribute{
				Description: "The tags, ordered by ID.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":    schema.StringAttribute{Computed: true, Description: "Numeric ID of the tag."},
						"name":  schema.StringAttribute{Computed: true, Description: "Name of the tag."},
						"color": schema.StringAttribute{Computed: true, Description: "Color of the tag."},
					},
				},
			},
		},
	}
}

func (d *ListDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	tags, err := d.client.ListTags(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list tags", err.Error())
		return
	}

	sort.Slice(tags, func(i, j int) bool { return tags[i].ID < tags[j].ID })

	entries := make([]ListEntry, 0, len(tags))
	for _, tag := range tags {
		entries = append(entries, ListEntry{
			ID:    types.StringValue(strconv.Itoa(tag.ID)),
			Name:  types.StringValue(tag.Name),
			Color: types.StringValue(tag.Color),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &ListModel{Tags: entries})...)
}
