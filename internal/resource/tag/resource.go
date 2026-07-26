package tag

import (
	"context"
	"strconv"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Resource manages a tag. Tags are global in Uptime Kuma, not per-user, and
// are attached to monitors through the monitor resources' `tags` attribute.
type Resource struct {
	client common.KumaClient
}

type Model struct {
	ID    types.String `tfsdk:"id"`
	Name  types.String `tfsdk:"name"`
	Color types.String `tfsdk:"color"`
}

var (
	_ resource.Resource                = (*Resource)(nil)
	_ resource.ResourceWithConfigure   = (*Resource)(nil)
	_ resource.ResourceWithImportState = (*Resource)(nil)
)

func New() resource.Resource {
	return &Resource{}
}

func (r *Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_tag"
}

func (r *Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, err := common.ConfigureClient(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected provider data", err.Error())
		return
	}
	r.client = client
}

func (r *Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A tag that can be attached to monitors. Tags are shared across the whole Uptime Kuma instance.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Numeric ID of the tag, assigned by Uptime Kuma.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Name of the tag.",
				Required:    true,
			},
			"color": schema.StringAttribute{
				Description: "Color used in the dashboard, as a CSS hex value such as `#4B5563`.",
				Required:    true,
			},
		},
	}
}

func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	var id int
	err := common.RetryRPC(ctx, 3, func() error {
		var err error
		id, err = r.client.CreateTag(ctx, kuma.Tag{
			Name:  model.Name.ValueString(),
			Color: model.Color.ValueString(),
		})
		return err
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to create tag", err.Error())
		return
	}

	model.ID = types.StringValue(strconv.Itoa(id))
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model Model
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, ok := common.ParseID(model.ID, &resp.Diagnostics)
	if !ok {
		return
	}

	tag, err := r.client.GetTag(ctx, id)
	if err != nil {
		if kuma.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read tag", err.Error())
		return
	}

	model.Name = types.StringValue(tag.Name)
	model.Color = types.StringValue(tag.Color)
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var model Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, ok := common.ParseID(model.ID, &resp.Diagnostics)
	if !ok {
		return
	}

	err := common.RetryRPC(ctx, 3, func() error {
		return r.client.UpdateTag(ctx, kuma.Tag{
			ID:    id,
			Name:  model.Name.ValueString(),
			Color: model.Color.ValueString(),
		})
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to update tag", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model Model
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, ok := common.ParseID(model.ID, &resp.Diagnostics)
	if !ok {
		return
	}

	// Deleting a tag also detaches it from every monitor that used it.
	err := common.RetryRPC(ctx, 3, func() error {
		return r.client.DeleteTag(ctx, id)
	})
	if err != nil && !kuma.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete tag", err.Error())
	}
}

func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
