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

// KeywordModel is an HTTP monitor that also searches the response body
// for a keyword.
type KeywordModel struct {
	BaseModel
	HTTPBase

	Keyword                     types.String `tfsdk:"keyword"`
	InvertKeyword               types.Bool   `tfsdk:"invert_keyword"`
	RetryOnlyOnStatusCodeFailed types.Bool   `tfsdk:"retry_only_on_status_code_failure"`
}

var _ Model = (*KeywordModel)(nil)

func (m *KeywordModel) Base() *BaseModel { return &m.BaseModel }

func (m *KeywordModel) ApplyTo(ctx context.Context, wire *kuma.Monitor, diags *diag.Diagnostics) {
	m.applyHTTPBase(ctx, wire, diags)
	wire.Keyword = common.StringPtr(m.Keyword)
	wire.InvertKeyword = common.BoolPtr(m.InvertKeyword)
	wire.RetryOnlyOnStatusCodeFailed = common.BoolPtr(m.RetryOnlyOnStatusCodeFailed)
}

func (m *KeywordModel) ReadFrom(ctx context.Context, wire *kuma.Monitor, diags *diag.Diagnostics) {
	m.readHTTPBase(ctx, wire, diags)
	m.Keyword = common.StringValue(wire.Keyword)
	m.InvertKeyword = common.BoolOrFalse(wire.InvertKeyword)
	m.RetryOnlyOnStatusCodeFailed = common.BoolOrFalse(wire.RetryOnlyOnStatusCodeFailed)
}

// NewKeywordResource returns the uptimekuma_monitor_keyword resource.
func NewKeywordResource() resource.Resource {
	attributes := httpAttributes()
	attributes["keyword"] = schema.StringAttribute{
		Description: "Keyword that must appear in the response body for the check to pass.",
		Required:    true,
	}
	attributes["invert_keyword"] = schema.BoolAttribute{
		Description: "Invert the match: the check passes when the keyword is absent. Default: false.",
		Optional:    true,
		Computed:    true,
	}
	attributes["retry_only_on_status_code_failure"] = schema.BoolAttribute{
		Description: "Retry only when the status code fails, not when the keyword is missing. Default: false.",
		Optional:    true,
		Computed:    true,
	}

	return New(TypeDef{
		TypeName:    "monitor_keyword",
		WireType:    "keyword",
		Description: "Monitors an HTTP(s) endpoint and requires a keyword to be present in the response body.",
		Attributes:  attributes,
		NewModel:    func() Model { return &KeywordModel{} },
	})()
}
