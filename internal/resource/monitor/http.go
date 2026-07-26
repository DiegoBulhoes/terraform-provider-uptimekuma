package monitor

import (
	"context"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// HTTPModel is a plain HTTP(s) monitor: success is decided by the status
// code alone.
type HTTPModel struct {
	BaseModel
	HTTPBase
}

var _ Model = (*HTTPModel)(nil)

func (m *HTTPModel) Base() *BaseModel { return &m.BaseModel }

func (m *HTTPModel) ApplyTo(ctx context.Context, wire *kuma.Monitor, diags *diag.Diagnostics) {
	m.applyHTTPBase(ctx, wire, diags)
}

func (m *HTTPModel) ReadFrom(ctx context.Context, wire *kuma.Monitor, diags *diag.Diagnostics) {
	m.readHTTPBase(ctx, wire, diags)
}

// NewHTTPResource returns the uptimekuma_monitor_http resource.
func NewHTTPResource() resource.Resource {
	return New(TypeDef{
		TypeName:    "monitor_http",
		WireType:    "http",
		Description: "Monitors an HTTP(s) endpoint, treating the response status code as the health signal.",
		Attributes:  httpAttributes(),
		NewModel:    func() Model { return &HTTPModel{} },
	})()
}
