// Package statuspage exposes status pages for reading, one by slug and the full
// list.
package statuspage

import (
	"context"
	"sort"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// DataSource looks up one status page by slug, including its groups.
type DataSource struct {
	client common.KumaClient
}

type Model struct {
	Slug   types.String `tfsdk:"slug"`
	PageID types.Int64  `tfsdk:"page_id"`
	Title  types.String `tfsdk:"title"`

	Description types.String `tfsdk:"description"`
	Theme       types.String `tfsdk:"theme"`
	Published   types.Bool   `tfsdk:"published"`
	ShowTags    types.Bool   `tfsdk:"show_tags"`

	DomainNames types.List `tfsdk:"domain_names"`
	Groups      types.List `tfsdk:"groups"`
}

var (
	_ datasource.DataSource              = (*DataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*DataSource)(nil)
)

func New() datasource.DataSource {
	return &DataSource{}
}

func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_page"
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

// groupObjectType describes a group as this data source reports it.
func groupObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"name":        types.StringType,
		"monitor_ids": types.ListType{ElemType: types.Int64Type},
	}}
}

func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Looks up a status page by slug, with the groups shown on it.",
		Attributes: map[string]schema.Attribute{
			"slug": schema.StringAttribute{
				Description: "Slug of the status page.",
				Required:    true,
			},
			"page_id": schema.Int64Attribute{
				Description: "Numeric ID of the page, for `uptimekuma_maintenance.status_page_ids`.",
				Computed:    true,
			},
			"title":       schema.StringAttribute{Computed: true, Description: "Title of the page."},
			"description": schema.StringAttribute{Computed: true, Description: "Description of the page."},
			"theme":       schema.StringAttribute{Computed: true, Description: "Page theme."},
			"published":   schema.BoolAttribute{Computed: true, Description: "Whether the page is published."},
			"show_tags":   schema.BoolAttribute{Computed: true, Description: "Whether monitor tags are shown."},
			"domain_names": schema.ListAttribute{
				Description: "Custom domains that serve this page.",
				Computed:    true,
				ElementType: types.StringType,
			},
			"groups": schema.ListNestedAttribute{
				Description: "Groups on the page, in display order.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{Computed: true, Description: "Heading of the group."},
						"monitor_ids": schema.ListAttribute{
							Description: "IDs of the monitors in the group, in display order.",
							Computed:    true,
							ElementType: types.Int64Type,
						},
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

	slug := model.Slug.ValueString()

	page, err := d.client.GetStatusPage(ctx, slug)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read status page", err.Error())
		return
	}
	groups, err := d.client.GetStatusPageGroups(ctx, page.Slug)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read the status page's groups", err.Error())
		return
	}

	model.Slug = types.StringValue(page.Slug)
	model.PageID = types.Int64Value(int64(page.ID))
	model.Title = types.StringValue(page.Title)
	model.Description = common.OptionalString(page.Description)
	model.Theme = types.StringValue(page.Theme)
	model.Published = common.BoolOrTrue(page.Published)
	model.ShowTags = common.BoolOrFalse(page.ShowTags)

	domains, domainDiags := types.ListValueFrom(ctx, types.StringType, page.DomainNameList)
	resp.Diagnostics.Append(domainDiags...)
	model.DomainNames = domains

	elements := make([]attr.Value, 0, len(groups))
	for _, group := range groups {
		ids := make([]attr.Value, 0, len(group.MonitorList))
		for _, monitor := range group.MonitorList {
			ids = append(ids, types.Int64Value(int64(monitor.ID)))
		}
		idList, idDiags := types.ListValue(types.Int64Type, ids)
		resp.Diagnostics.Append(idDiags...)

		object, objectDiags := types.ObjectValue(groupObjectType().AttrTypes, map[string]attr.Value{
			"name":        types.StringValue(group.Name),
			"monitor_ids": idList,
		})
		resp.Diagnostics.Append(objectDiags...)
		elements = append(elements, object)
	}
	groupList, groupDiags := types.ListValue(groupObjectType(), elements)
	resp.Diagnostics.Append(groupDiags...)
	model.Groups = groupList

	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// ── List ────────────────────────────────────────────────────────────

type ListDataSource struct {
	client common.KumaClient
}

type ListModel struct {
	StatusPages []ListEntry `tfsdk:"status_pages"`
}

type ListEntry struct {
	PageID    types.Int64  `tfsdk:"page_id"`
	Slug      types.String `tfsdk:"slug"`
	Title     types.String `tfsdk:"title"`
	Published types.Bool   `tfsdk:"published"`
}

var (
	_ datasource.DataSource              = (*ListDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*ListDataSource)(nil)
)

func NewList() datasource.DataSource {
	return &ListDataSource{}
}

func (d *ListDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_pages"
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
		Description: "Lists the status pages on the instance.",
		MarkdownDescription: "Lists the status pages on the instance.\n\n" +
			"~> Uptime Kuma sends this list only once, when a client logs in, and no mutation re-sends it. To get a " +
			"current list the provider reconnects, which spends one login against the server's limit of 20 per " +
			"minute. Reading a single page with `uptimekuma_status_page` does not have that cost.",
		Attributes: map[string]schema.Attribute{
			"status_pages": schema.ListNestedAttribute{
				Description: "The status pages, ordered by ID.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"page_id":   schema.Int64Attribute{Computed: true, Description: "Numeric ID of the page."},
						"slug":      schema.StringAttribute{Computed: true, Description: "Slug of the page."},
						"title":     schema.StringAttribute{Computed: true, Description: "Title of the page."},
						"published": schema.BoolAttribute{Computed: true, Description: "Whether the page is published."},
					},
				},
			},
		},
	}
}

func (d *ListDataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	// refresh=true: the cached list dates from login, so a page created earlier
	// in the same run would otherwise be missing.
	pages, err := d.client.ListStatusPages(ctx, true)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list status pages", err.Error())
		return
	}

	ids := make([]int, 0, len(pages))
	for id := range pages {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	entries := make([]ListEntry, 0, len(ids))
	for _, id := range ids {
		page := pages[id]
		entries = append(entries, ListEntry{
			PageID:    types.Int64Value(int64(page.ID)),
			Slug:      types.StringValue(page.Slug),
			Title:     types.StringValue(page.Title),
			Published: common.BoolOrTrue(page.Published),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &ListModel{StatusPages: entries})...)
}
