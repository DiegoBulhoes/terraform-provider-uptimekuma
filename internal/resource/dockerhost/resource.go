package dockerhost

import (
	"context"
	"strconv"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Resource manages a Docker daemon that docker monitors query.
type Resource struct {
	client common.KumaClient
}

type Model struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	ConnectionType types.String `tfsdk:"connection_type"`
	Daemon         types.String `tfsdk:"daemon"`
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
	resp.TypeName = req.ProviderTypeName + "_docker_host"
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
		Description: "A Docker daemon that `uptimekuma_monitor_docker` resources query for container state.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Numeric ID of the Docker host, assigned by Uptime Kuma.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Friendly name for the Docker host.",
				Required:    true,
			},
			"connection_type": schema.StringAttribute{
				Description: "How to reach the daemon: `socket` for a Unix socket or `tcp` for a TCP/HTTP endpoint.",
				Required:    true,
				Validators: []validator.String{
					stringvalidator.OneOf("socket", "tcp"),
				},
			},
			"daemon": schema.StringAttribute{
				Description: "Socket path or TCP address of the daemon, for example `/var/run/docker.sock` or `tcp://localhost:2375`.",
				Required:    true,
			},
		},
	}
}

func (m *Model) wire() kuma.DockerHost {
	return kuma.DockerHost{
		Name:         m.Name.ValueString(),
		DockerType:   m.ConnectionType.ValueString(),
		DockerDaemon: m.Daemon.ValueString(),
	}
}

func (m *Model) readInto(host *kuma.DockerHost) {
	m.ID = types.StringValue(strconv.Itoa(host.ID))
	m.Name = types.StringValue(host.Name)
	m.ConnectionType = types.StringValue(host.DockerType)
	m.Daemon = types.StringValue(host.DockerDaemon)
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
		id, err = r.client.SaveDockerHost(ctx, nil, model.wire())
		return err
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to create Docker host", err.Error())
		return
	}

	host, err := r.client.GetDockerHost(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Docker host back after creating it", err.Error())
		return
	}
	model.readInto(host)
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

	host, err := r.client.GetDockerHost(ctx, id)
	if err != nil {
		if kuma.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read Docker host", err.Error())
		return
	}
	model.readInto(host)
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
		_, err := r.client.SaveDockerHost(ctx, &id, model.wire())
		return err
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to update Docker host", err.Error())
		return
	}

	host, err := r.client.GetDockerHost(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Docker host back after updating it", err.Error())
		return
	}
	model.readInto(host)
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
		return r.client.DeleteDockerHost(ctx, id)
	})
	if err != nil && !kuma.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete Docker host", err.Error())
	}
}

func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
