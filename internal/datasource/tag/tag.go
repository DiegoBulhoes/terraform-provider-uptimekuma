// Package tag exposes tags for reading, one by id or name and the full list.
package tag

import (
	"context"
	"fmt"
	"strconv"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// DataSource looks up a tag by ID or name.
type DataSource struct {
	client common.KumaClient
}

type Model struct {
	ID    types.String `tfsdk:"id"`
	Name  types.String `tfsdk:"name"`
	Color types.String `tfsdk:"color"`
}

var (
	_ datasource.DataSource              = (*DataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*DataSource)(nil)
)

func New() datasource.DataSource {
	return &DataSource{}
}

func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tag"
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
		Description: "Looks up a tag by ID or by name.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Numeric ID of the tag. Provide either this or `name`.",
				Optional:    true,
				Computed:    true,
			},
			"name": schema.StringAttribute{
				Description: "Name of the tag. Provide either this or `id`. Must match exactly one tag.",
				Optional:    true,
				Computed:    true,
			},
			"color": schema.StringAttribute{
				Description: "Color of the tag, as a CSS hex value.",
				Computed:    true,
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
		resp.Diagnostics.AddError("Ambiguous tag lookup", "Set either `id` or `name`, not both.")
		return
	case !hasID && !hasName:
		resp.Diagnostics.AddError("Missing tag lookup", "Set either `id` or `name`.")
		return
	}

	tags, err := d.client.ListTags(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list tags", err.Error())
		return
	}

	if hasID {
		id, ok := common.ParseID(model.ID, &resp.Diagnostics)
		if !ok {
			return
		}
		for _, tag := range tags {
			if tag.ID == id {
				model.Name = types.StringValue(tag.Name)
				model.Color = types.StringValue(tag.Color)
				resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
				return
			}
		}
		resp.Diagnostics.AddError("Tag not found", fmt.Sprintf("No tag with ID %d.", id))
		return
	}

	name := model.Name.ValueString()
	matches := 0
	for _, tag := range tags {
		if tag.Name != name {
			continue
		}
		matches++
		model.ID = types.StringValue(strconv.Itoa(tag.ID))
		model.Color = types.StringValue(tag.Color)
	}
	switch matches {
	case 0:
		resp.Diagnostics.AddError("Tag not found", fmt.Sprintf("No tag named %q.", name))
	case 1:
		resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
	default:
		// Uptime Kuma allows duplicate tag names, so this is reachable.
		resp.Diagnostics.AddError(
			"Ambiguous tag name",
			fmt.Sprintf("%d tags are named %q; look the tag up by id instead.", matches, name),
		)
	}
}
