// Package tag exposes tags for reading, one by id or name and the full list.
package tag

import (
	"context"
	"fmt"
	"strconv"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
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

	if !validLookup(model, &resp.Diagnostics) {
		return
	}

	var tags []kuma.Tag
	err := common.RetryRPC(ctx, 3, func() error {
		var err error
		tags, err = d.client.ListTags(ctx)
		return err
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to list tags", err.Error())
		return
	}

	var found *kuma.Tag
	if common.IsSet(model.ID) {
		found = d.findByID(model, tags, &resp.Diagnostics)
	} else {
		found = d.findByName(model, tags, &resp.Diagnostics)
	}
	if found == nil {
		return
	}

	model.ID = types.StringValue(strconv.Itoa(found.ID))
	model.Name = types.StringValue(found.Name)
	model.Color = types.StringValue(found.Color)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

// validLookup checks exactly one of id and name was given.
func validLookup(model Model, diags *diag.Diagnostics) bool {
	hasID := common.IsSet(model.ID)
	hasName := common.IsSet(model.Name)
	switch {
	case hasID && hasName:
		diags.AddError("Ambiguous tag lookup", "Set either `id` or `name`, not both.")
		return false
	case !hasID && !hasName:
		diags.AddError("Missing tag lookup", "Set either `id` or `name`.")
		return false
	}
	return true
}

func (d *DataSource) findByID(model Model, tags []kuma.Tag, diags *diag.Diagnostics) *kuma.Tag {
	id, ok := common.ParseID(model.ID, diags)
	if !ok {
		return nil
	}
	for _, tag := range tags {
		if tag.ID == id {
			return &tag
		}
	}
	diags.AddError("Tag not found", fmt.Sprintf("No tag with ID %d.", id))
	return nil
}

// findByName requires exactly one match. Uptime Kuma allows duplicate tag names,
// so picking one silently would be a coin toss.
func (d *DataSource) findByName(model Model, tags []kuma.Tag, diags *diag.Diagnostics) *kuma.Tag {
	name := model.Name.ValueString()

	var matches []kuma.Tag
	for _, tag := range tags {
		if tag.Name == name {
			matches = append(matches, tag)
		}
	}

	switch len(matches) {
	case 0:
		diags.AddError("Tag not found", fmt.Sprintf("No tag named %q.", name))
		return nil
	case 1:
		return &matches[0]
	default:
		diags.AddError(
			"Ambiguous tag name",
			fmt.Sprintf("%d tags are named %q; look the tag up by id instead.", len(matches), name),
		)
		return nil
	}
}
