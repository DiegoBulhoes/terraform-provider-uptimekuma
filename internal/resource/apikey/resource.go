package apikey

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

// Resource manages an API key.
//
// These keys authenticate the Prometheus `/metrics` endpoint, not the Socket.IO
// API this provider speaks — so creating one does not give Terraform another way
// to connect.
type Resource struct {
	client common.KumaClient
}

type Model struct {
	ID      types.String `tfsdk:"id"`
	Name    types.String `tfsdk:"name"`
	Active  types.Bool   `tfsdk:"active"`
	Expires types.String `tfsdk:"expires"`
	Key     types.String `tfsdk:"key"`
	Status  types.String `tfsdk:"status"`
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
	resp.TypeName = req.ProviderTypeName + "_api_key"
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
		Description: "An API key for the Prometheus `/metrics` endpoint. Creating the first key also switches the " +
			"server's `apiKeysEnabled` setting on.",
		MarkdownDescription: "An API key for the Prometheus `/metrics` endpoint. Creating the first key also " +
			"switches the server's `apiKeysEnabled` setting on.\n\n" +
			"~> **The key is only returned once.** Uptime Kuma stores just a hash, so `key` is available in state " +
			"from the moment of creation and can never be recovered afterwards — an imported key has a null `key`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Numeric ID of the API key, assigned by Uptime Kuma.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Friendly name for the key.",
				Required:    true,
				// There is no edit event for API keys, only enable and disable,
				// so any other change has to go through a replacement.
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"active": schema.BoolAttribute{
				Description: "Whether the key is enabled. Default: true.",
				Optional:    true,
				Computed:    true,
			},
			"expires": schema.StringAttribute{
				Description: "Expiry timestamp, as `2026-12-31 23:59:00`. Leave unset for a key that never expires.",
				Optional:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplace(),
				},
			},
			"key": schema.StringAttribute{
				Description: "The generated key, in the form `uk<id>_<secret>`. Only ever returned at creation time.",
				Computed:    true,
				Sensitive:   true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"status": schema.StringAttribute{
				Description: "Server-computed status: `active`, `inactive` or `expired`.",
				Computed:    true,
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

	active := !common.IsSet(model.Active) || model.Active.ValueBool()

	var (
		id       int
		clearKey string
	)
	err := common.RetryRPC(ctx, 3, func() error {
		var err error
		id, clearKey, err = r.client.CreateAPIKey(ctx, kuma.APIKey{
			Name:    model.Name.ValueString(),
			Active:  kuma.Bool(active),
			Expires: common.StringPtr(model.Expires),
		})
		return err
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to create API key", err.Error())
		return
	}

	// The clear-text key exists only in this response, so it is stored now or
	// lost for good.
	model.ID = types.StringValue(strconv.Itoa(id))
	model.Key = types.StringValue(clearKey)

	key, err := r.client.GetAPIKey(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read API key back after creating it", err.Error())
		return
	}
	model.Active = types.BoolValue(bool(key.Active))
	model.Status = types.StringValue(key.Status)

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

	key, err := r.client.GetAPIKey(ctx, id)
	if err != nil {
		if kuma.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read API key", err.Error())
		return
	}

	model.Name = types.StringValue(key.Name)
	model.Active = types.BoolValue(bool(key.Active))
	model.Status = types.StringValue(key.Status)
	model.Expires = common.OptionalString(key.Expires)
	// model.Key is deliberately left as it was: the server never returns it
	// again, and overwriting it would destroy the only copy.

	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	var state Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, ok := common.ParseID(state.ID, &resp.Diagnostics)
	if !ok {
		return
	}

	// Only `active` can change in place; name and expiry force a replacement.
	if plan.Active.ValueBool() != state.Active.ValueBool() {
		err := common.RetryRPC(ctx, 3, func() error {
			return r.client.SetAPIKeyActive(ctx, id, plan.Active.ValueBool())
		})
		if err != nil {
			resp.Diagnostics.AddError("Unable to change the API key's active state", err.Error())
			return
		}
	}

	key, err := r.client.GetAPIKey(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read API key back after updating it", err.Error())
		return
	}

	plan.Key = state.Key
	plan.Active = types.BoolValue(bool(key.Active))
	plan.Status = types.StringValue(key.Status)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
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

	err := common.RetryRPC(ctx, 3, func() error {
		return r.client.DeleteAPIKey(ctx, id)
	})
	if err != nil && !kuma.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete API key", err.Error())
	}
}

func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
	// `key` stays null after an import; the server only ever hands out the
	// clear-text value at creation.
}
