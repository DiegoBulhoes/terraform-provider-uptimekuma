package proxy

import (
	"context"
	"strconv"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Resource manages an outbound proxy that HTTP monitors can route through.
type Resource struct {
	client common.KumaClient
}

type Model struct {
	ID            types.String `tfsdk:"id"`
	Protocol      types.String `tfsdk:"protocol"`
	Host          types.String `tfsdk:"host"`
	Port          types.Int64  `tfsdk:"port"`
	Username      types.String `tfsdk:"username"`
	Password      types.String `tfsdk:"password"`
	Active        types.Bool   `tfsdk:"active"`
	Default       types.Bool   `tfsdk:"default"`
	ApplyExisting types.Bool   `tfsdk:"apply_existing"`
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
	resp.TypeName = req.ProviderTypeName + "_proxy"
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
		Description: "An outbound proxy that HTTP-based monitors can send their requests through.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Numeric ID of the proxy, assigned by Uptime Kuma.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"protocol": schema.StringAttribute{
				Description: "Proxy protocol: `http`, `https`, `socks`, `socks4` or `socks5`.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.OneOf("http", "https", "socks", "socks4", "socks5", "socks5h"),
				},
			},
			"host": schema.StringAttribute{
				Description: "Proxy hostname or IP address.",
				Required:    true,
			},
			"port": schema.Int64Attribute{
				Description: "Proxy port.",
				Required:    true,
				Validators: []validator.Int64{
					int64validator.Between(1, 65535),
				},
			},
			"username": schema.StringAttribute{
				Description: "Username for proxy authentication. Setting this or `password` enables authentication.",
				Optional:    true,
			},
			"password": schema.StringAttribute{
				Description: "Password for proxy authentication.",
				Optional:    true,
				Sensitive:   true,
			},
			"active": schema.BoolAttribute{
				Description: "Whether the proxy can be used. Default: true.",
				Optional:    true,
				Computed:    true,
			},
			"default": schema.BoolAttribute{
				Description: "Use this proxy by default for new monitors. Setting it deactivates the previous default. Default: false.",
				Optional:    true,
				Computed:    true,
			},
			"apply_existing": schema.BoolAttribute{
				Description: "When true, apply this proxy to every existing monitor at apply time. " +
					"This is a one-shot server action, not stored state, so Terraform cannot detect drift in it.",
				Optional: true,
			},
		},
	}
}

func (m *Model) wire() kuma.Proxy {
	// The server keeps `auth` as its own column; deriving it from the presence of
	// credentials means one less thing for the user to keep consistent.
	auth := common.IsSet(m.Username) || common.IsSet(m.Password)

	return kuma.Proxy{
		Protocol:      m.Protocol.ValueString(),
		Host:          m.Host.ValueString(),
		Port:          int(m.Port.ValueInt64()),
		Auth:          kuma.Bool(auth),
		Username:      common.StringPtr(m.Username),
		Password:      common.StringPtr(m.Password),
		Active:        kuma.Bool(!common.IsSet(m.Active) || m.Active.ValueBool()),
		Default:       kuma.Bool(common.IsSet(m.Default) && m.Default.ValueBool()),
		ApplyExisting: kuma.Bool(common.IsSet(m.ApplyExisting) && m.ApplyExisting.ValueBool()),
	}
}

func (m *Model) readInto(proxy *kuma.Proxy) {
	m.ID = types.StringValue(strconv.Itoa(proxy.ID))
	m.Protocol = types.StringValue(proxy.Protocol)
	m.Host = types.StringValue(proxy.Host)
	m.Port = types.Int64Value(int64(proxy.Port))
	m.Username = common.OptionalString(proxy.Username)
	m.Password = common.OptionalString(proxy.Password)
	m.Active = types.BoolValue(bool(proxy.Active))
	m.Default = types.BoolValue(bool(proxy.Default))
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
		id, err = r.client.SaveProxy(ctx, nil, model.wire())
		return err
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to create proxy", err.Error())
		return
	}

	proxy, err := r.client.GetProxy(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read proxy back after creating it", err.Error())
		return
	}

	applyExisting := model.ApplyExisting
	model.readInto(proxy)
	model.ApplyExisting = applyExisting
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

	proxy, err := r.client.GetProxy(ctx, id)
	if err != nil {
		if kuma.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read proxy", err.Error())
		return
	}

	applyExisting := model.ApplyExisting
	model.readInto(proxy)
	model.ApplyExisting = applyExisting
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
		_, err := r.client.SaveProxy(ctx, &id, model.wire())
		return err
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to update proxy", err.Error())
		return
	}

	proxy, err := r.client.GetProxy(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read proxy back after updating it", err.Error())
		return
	}

	applyExisting := model.ApplyExisting
	model.readInto(proxy)
	model.ApplyExisting = applyExisting
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

	err := common.RetryRPC(ctx, 3, func() error {
		return r.client.DeleteProxy(ctx, id)
	})
	if err != nil && !kuma.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete proxy", err.Error())
	}
}

func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
