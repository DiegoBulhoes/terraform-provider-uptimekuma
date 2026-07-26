package monitor

import (
	"context"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// DockerModel watches a container through a Docker daemon.
type DockerModel struct {
	BaseModel

	ContainerName types.String `tfsdk:"container_name"`
	DockerHostID  types.Int64  `tfsdk:"docker_host_id"`
}

var _ Model = (*DockerModel)(nil)

func (m *DockerModel) Base() *BaseModel { return &m.BaseModel }

func (m *DockerModel) ApplyTo(ctx context.Context, wire *kuma.Monitor, diags *diag.Diagnostics) {
	wire.DockerContainer = common.StringPtr(m.ContainerName)
	wire.DockerHost = common.IntPtr(m.DockerHostID)
	_ = ctx
	_ = diags
}

func (m *DockerModel) ReadFrom(ctx context.Context, wire *kuma.Monitor, diags *diag.Diagnostics) {
	m.ContainerName = common.StringValue(wire.DockerContainer)
	m.DockerHostID = common.IntValue(wire.DockerHost)
	_ = ctx
	_ = diags
}

// NewDockerResource returns the uptimekuma_monitor_docker resource.
func NewDockerResource() resource.Resource {
	return New(TypeDef{
		TypeName:    "monitor_docker",
		WireType:    "docker",
		Description: "Monitors a Docker container's running state through a configured Docker host.",
		Attributes: map[string]schema.Attribute{
			"container_name": schema.StringAttribute{
				Description: "Name or ID of the container to watch.",
				Required:    true,
			},
			"docker_host_id": schema.Int64Attribute{
				Description: "ID of the Docker host to query, from `uptimekuma_docker_host`.",
				Required:    true,
			},
		},
		NewModel: func() Model { return &DockerModel{} },
	})()
}
