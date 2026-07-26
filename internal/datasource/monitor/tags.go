package monitor

import (
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// tagObjectType describes a monitor's tag as the data source exposes it.
//
// It carries the tag's name and color as well, which the resource attribute
// does not: a data source only reads, so the extra context costs nothing and
// saves a second lookup.
func tagObjectType() types.ObjectType {
	return types.ObjectType{AttrTypes: map[string]attr.Type{
		"tag_id": types.Int64Type,
		"name":   types.StringType,
		"color":  types.StringType,
		"value":  types.StringType,
	}}
}

func tagSet(tags []kuma.MonitorTag, diags *diag.Diagnostics) types.Set {
	if len(tags) == 0 {
		return types.SetNull(tagObjectType())
	}

	elements := make([]attr.Value, 0, len(tags))
	for _, tag := range tags {
		object, objectDiags := types.ObjectValue(tagObjectType().AttrTypes, map[string]attr.Value{
			"tag_id": types.Int64Value(int64(tag.TagID)),
			"name":   types.StringValue(tag.Name),
			"color":  types.StringValue(tag.Color),
			"value":  common.OptionalString(tag.Value),
		})
		diags.Append(objectDiags...)
		elements = append(elements, object)
	}

	set, setDiags := types.SetValue(tagObjectType(), elements)
	diags.Append(setDiags...)
	return set
}
