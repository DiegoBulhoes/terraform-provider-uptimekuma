package monitor

import (
	"context"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// JSONQueryModel is an HTTP monitor that evaluates a JSON path against
// the response body.
type JSONQueryModel struct {
	BaseModel
	HTTPBase

	JSONPath         types.String `tfsdk:"json_path"`
	JSONPathOperator types.String `tfsdk:"json_path_operator"`
	ExpectedValue    types.String `tfsdk:"expected_value"`
}

var _ Model = (*JSONQueryModel)(nil)

func (m *JSONQueryModel) Base() *BaseModel { return &m.BaseModel }

func (m *JSONQueryModel) ApplyTo(ctx context.Context, wire *kuma.Monitor, diags *diag.Diagnostics) {
	m.applyHTTPBase(ctx, wire, diags)
	wire.JSONPath = common.StringPtr(m.JSONPath)
	wire.JSONPathOperator = common.StringPtr(m.JSONPathOperator)
	wire.ExpectedValue = common.StringPtr(m.ExpectedValue)
}

func (m *JSONQueryModel) ReadFrom(ctx context.Context, wire *kuma.Monitor, diags *diag.Diagnostics) {
	m.readHTTPBase(ctx, wire, diags)
	m.JSONPath = common.StringValue(wire.JSONPath)
	m.JSONPathOperator = common.StringValue(wire.JSONPathOperator)
	m.ExpectedValue = common.StringValue(wire.ExpectedValue)
}

// NewJSONQueryResource returns the uptimekuma_monitor_json_query resource.
func NewJSONQueryResource() resource.Resource {
	attributes := httpAttributes()
	attributes["json_path"] = schema.StringAttribute{
		Description: "JSON path (jsonata syntax) selecting the value to compare, for example `status`.",
		Required:    true,
	}
	attributes["json_path_operator"] = schema.StringAttribute{
		Description: "Comparison operator applied to the selected value.",
		Optional:    true,
		Computed:    true,
		Validators: []validator.String{
			stringvalidator.OneOf("==", "!=", "<", "<=", ">", ">=", "contains"),
		},
	}
	attributes["expected_value"] = schema.StringAttribute{
		Description: "Value the selected JSON path is compared against.",
		Required:    true,
	}

	return New(TypeDef{
		TypeName:    "monitor_json_query",
		WireType:    "json-query",
		Description: "Monitors an HTTP(s) endpoint and compares a value extracted from the JSON response against an expected value.",
		Attributes:  attributes,
		NewModel:    func() Model { return &JSONQueryModel{} },
	})()
}
