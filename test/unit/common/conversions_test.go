package common_test

import (
	"context"
	"testing"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The remaining branches of the conversion helpers. Each one is small, but they
// sit between Terraform and the wire on every single attribute, so a wrong branch
// shows up as an attribute that silently loses its value.

func TestParseID(t *testing.T) {
	t.Parallel()

	t.Run("a numeric string parses", func(t *testing.T) {
		t.Parallel()

		var diagnostics diag.Diagnostics
		got, ok := common.ParseID(types.StringValue("42"), &diagnostics)
		if !ok || got != 42 {
			t.Errorf("got %d, ok=%v", got, ok)
		}
		if diagnostics.HasError() {
			t.Errorf("unexpected diagnostics: %s", diagnostics)
		}
	})

	for _, invalid := range []string{"", "abc", "1.5", "12x", " 12"} {
		t.Run("rejects "+invalid, func(t *testing.T) {
			t.Parallel()

			var diagnostics diag.Diagnostics
			if _, ok := common.ParseID(types.StringValue(invalid), &diagnostics); ok {
				t.Errorf("%q should not parse", invalid)
			}
			if !diagnostics.HasError() {
				t.Error("a rejection should explain itself")
			}
		})
	}
}

func TestOptionalString(t *testing.T) {
	t.Parallel()

	if got := common.OptionalString(nil); !got.IsNull() {
		t.Error("nil should be null")
	}

	empty := ""
	// Uptime Kuma stores an unset text field as "", while Terraform models it as
	// null; collapsing the two is what keeps plans clean.
	if got := common.OptionalString(&empty); !got.IsNull() {
		t.Error("an empty string should be null")
	}

	value := "x"
	if got := common.OptionalString(&value); got.ValueString() != "x" {
		t.Errorf("got %q", got.ValueString())
	}
}

func TestBoolDefaults(t *testing.T) {
	t.Parallel()

	// Absent means different things for different fields, which is why there are
	// two helpers.
	if got := common.BoolOrTrue(nil); !got.ValueBool() {
		t.Error("BoolOrTrue(nil) should be true")
	}
	if got := common.BoolOrTrue(kuma.BoolPtr(false)); got.ValueBool() {
		t.Error("an explicit false must win over the default")
	}
	if got := common.BoolOrFalse(nil); got.ValueBool() {
		t.Error("BoolOrFalse(nil) should be false")
	}
	if got := common.BoolOrFalse(kuma.BoolPtr(true)); !got.ValueBool() {
		t.Error("an explicit true must win over the default")
	}
}

func TestFloat64Value(t *testing.T) {
	t.Parallel()

	if got := common.Float64Value(nil); !got.IsNull() {
		t.Error("nil should be null")
	}
	value := 2.5
	if got := common.Float64Value(&value); got.ValueFloat64() != 2.5 {
		t.Errorf("got %v", got.ValueFloat64())
	}
}

func TestInt64Set(t *testing.T) {
	t.Parallel()

	var diagnostics diag.Diagnostics

	set := common.Int64Set([]int{3, 1, 2}, &diagnostics)
	if diagnostics.HasError() {
		t.Fatalf("diagnostics: %s", diagnostics)
	}
	if len(set.Elements()) != 3 {
		t.Errorf("got %d elements", len(set.Elements()))
	}

	// An empty slice is a set with nothing in it, not null: the two mean
	// different things when the server replaces an association wholesale.
	empty := common.Int64Set(nil, &diagnostics)
	if empty.IsNull() {
		t.Error("an empty slice should produce an empty set, not null")
	}
	if len(empty.Elements()) != 0 {
		t.Errorf("got %d elements", len(empty.Elements()))
	}
}

func TestStringListToSlice(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	list, diagnostics := types.ListValue(types.StringType, []attr.Value{
		types.StringValue("status.example.com"),
		types.StringValue("uptime.example.com"),
	})
	if diagnostics.HasError() {
		t.Fatalf("building the list: %s", diagnostics)
	}

	got := common.StringListToSlice(ctx, list)
	if len(got) != 2 || got[0] != "status.example.com" {
		t.Errorf("got %v", got)
	}

	// Order matters here, unlike a set: this feeds a status page's domain list.
	if got[1] != "uptime.example.com" {
		t.Errorf("order was lost: %v", got)
	}

	if got := common.StringListToSlice(ctx, types.ListNull(types.StringType)); got != nil {
		t.Errorf("null should produce nil, got %v", got)
	}
	if got := common.StringListToSlice(ctx, types.ListUnknown(types.StringType)); got != nil {
		t.Errorf("unknown should produce nil, got %v", got)
	}
}

func TestConfigureClientRejectsNil(t *testing.T) {
	t.Parallel()

	// A nil provider data means Configure ran before the provider was configured,
	// which the resources guard against separately; here it must simply not be
	// mistaken for a usable client.
	if _, err := common.ConfigureClient(nil); err == nil {
		t.Error("nil should be rejected")
	}
	if _, err := common.ConfigureClient(42); err == nil {
		t.Error("a wrong type should be rejected")
	}
}
