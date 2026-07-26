package resource_test

import (
	"context"
	"strings"
	"testing"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	kumaresource "github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/monitor"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// Every monitor type contributes an ApplyTo/ReadFrom pair: the two functions
// that translate its own attributes to and from the wire payload. There are 33
// of them and they are the easiest place in the provider to lose a field —
// writing one in ApplyTo and forgetting it in ReadFrom produces a permanent
// diff that no compiler catches.
//
// Rather than 33 acceptance tests, this drives the hooks directly and asserts
// the round trip: attributes -> wire -> attributes has to come back unchanged.
// Walking the registry means a type added later is covered for free.

// jsonAttributes hold serialized JSON, so a plain "x" would be rejected by the
// hook that parses them.
var jsonAttributes = map[string]string{
	"conditions":                  `[]`,
	"kafka_producer_brokers":      `["localhost:9092"]`,
	"kafka_producer_sasl_options": `{}`,
	"rabbitmq_nodes":              `["amqp://localhost"]`,
	"json_path":                   `$.status`,
	"headers":                     `{}`,
	"body":                        `{}`,
}

func monitorResources(t *testing.T) map[string]*monitor.Resource {
	t.Helper()

	ctx := context.Background()
	out := map[string]*monitor.Resource{}

	for _, factory := range kumaresource.All() {
		res, ok := factory().(*monitor.Resource)
		if !ok {
			continue // not a monitor type
		}
		metadataResp := &fwresource.MetadataResponse{}
		res.Metadata(ctx, fwresource.MetadataRequest{ProviderTypeName: "uptimekuma"}, metadataResp)
		out[metadataResp.TypeName] = res
	}

	if len(out) == 0 {
		t.Fatal("no monitor resources found in the registry")
	}
	return out
}

func schemaOf(t *testing.T, res *monitor.Resource) fwresource.SchemaResponse {
	t.Helper()

	resp := fwresource.SchemaResponse{}
	res.Schema(context.Background(), fwresource.SchemaRequest{}, &resp)
	if resp.Diagnostics.HasError() {
		t.Fatalf("schema: %s", resp.Diagnostics)
	}
	return resp
}

// plausible builds a value of the given type that a hook will accept.
func plausible(name string, t attr.Type) tftypes.Value {
	tfType := t.TerraformType(context.Background())

	switch {
	case tfType.Is(tftypes.String):
		if literal, special := jsonAttributes[name]; special {
			return tftypes.NewValue(tftypes.String, literal)
		}
		return tftypes.NewValue(tftypes.String, "x")
	case tfType.Is(tftypes.Bool):
		return tftypes.NewValue(tftypes.Bool, true)
	case tfType.Is(tftypes.Number):
		return tftypes.NewValue(tftypes.Number, 1)
	}

	switch container := tfType.(type) {
	case tftypes.Set:
		return tftypes.NewValue(container, []tftypes.Value{plausible(name, elementType(t))})
	case tftypes.List:
		return tftypes.NewValue(container, []tftypes.Value{plausible(name, elementType(t))})
	case tftypes.Map:
		return tftypes.NewValue(container, map[string]tftypes.Value{"k": plausible(name, elementType(t))})
	case tftypes.Object:
		attributes := make(map[string]tftypes.Value, len(container.AttributeTypes))
		for attributeName, attributeType := range container.AttributeTypes {
			attributes[attributeName] = plausibleRaw(attributeName, attributeType)
		}
		return tftypes.NewValue(container, attributes)
	}

	return tftypes.NewValue(tfType, nil)
}

// plausibleRaw is plausible for a bare tftypes.Type, used inside objects where
// the attr.Type is no longer at hand.
func plausibleRaw(name string, t tftypes.Type) tftypes.Value {
	switch {
	case t.Is(tftypes.String):
		if literal, special := jsonAttributes[name]; special {
			return tftypes.NewValue(tftypes.String, literal)
		}
		return tftypes.NewValue(tftypes.String, "x")
	case t.Is(tftypes.Bool):
		return tftypes.NewValue(tftypes.Bool, true)
	case t.Is(tftypes.Number):
		return tftypes.NewValue(tftypes.Number, 1)
	}
	return tftypes.NewValue(t, nil)
}

func elementType(t attr.Type) attr.Type {
	if withElement, ok := t.(attr.TypeWithElementType); ok {
		return withElement.ElementType()
	}
	return t
}

// nullObject builds a fully-null value of the schema's object type. The
// framework refuses to read into a model from an object whose attributes are
// missing entirely, so every attribute has to be present even when null.
func nullObject(t *testing.T, objectType tftypes.Object) tftypes.Value {
	t.Helper()

	attributes := make(map[string]tftypes.Value, len(objectType.AttributeTypes))
	for name, attributeType := range objectType.AttributeTypes {
		attributes[name] = tftypes.NewValue(attributeType, nil)
	}
	return tftypes.NewValue(objectType, attributes)
}

func TestEveryMonitorTypeRoundTripsItsAttributes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	for name, res := range monitorResources(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			schemaResp := schemaOf(t, res)
			objectType, ok := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
			if !ok {
				t.Fatal("a resource schema is always an object")
			}

			// Type-specific attributes only: the base ones are exercised by the
			// shared CRUD tests.
			specific := res.DefForTest().Attributes

			attributes := make(map[string]tftypes.Value, len(objectType.AttributeTypes))
			for attributeName, attributeType := range objectType.AttributeTypes {
				attribute, isSpecific := specific[attributeName]
				if !isSpecific {
					attributes[attributeName] = tftypes.NewValue(attributeType, nil)
					continue
				}
				attributes[attributeName] = plausible(attributeName, attribute.GetType())
			}
			filled := tftypes.NewValue(objectType, attributes)

			model := res.NewModelForTest()
			plan := tfsdk.Plan{Schema: schemaResp.Schema, Raw: filled}
			if diags := plan.Get(ctx, model); diags.HasError() {
				t.Fatalf("reading the plan into the model: %s", diags)
			}

			wire := newWireMonitor(res.DefForTest().WireType)
			applyDiags := diagnostics()
			model.ApplyTo(ctx, wire, applyDiags)
			if applyDiags.HasError() {
				t.Fatalf("ApplyTo rejected plausible values: %s", applyDiags)
			}

			// Read the same payload back into a fresh model. ReadFrom only writes
			// the type-specific attributes, so the base ones are copied over —
			// otherwise they stay at their zero value, which the framework
			// rejects as an untyped collection when serializing.
			readBack := res.NewModelForTest()
			*readBack.Base() = *model.Base()
			readDiags := diagnostics()
			readBack.ReadFrom(ctx, wire, readDiags)
			if readDiags.HasError() {
				t.Fatalf("ReadFrom failed on a payload ApplyTo produced: %s", readDiags)
			}

			// Serialize both models and compare only the type-specific attributes.
			before := serialize(t, schemaResp, model)
			after := serialize(t, schemaResp, readBack)

			for attributeName := range specific {
				attribute := specific[attributeName]
				if attribute.IsComputed() && !attribute.IsOptional() {
					// Computed-only attributes are server state; the plan value is
					// meaningless and ReadFrom is the only writer.
					continue
				}
				if !before[attributeName].Equal(after[attributeName]) {
					t.Errorf("%s does not survive the round trip:\n  applied:   %s\n  read back: %s\n"+
						"ApplyTo and ReadFrom disagree about this attribute, which is a permanent diff",
						attributeName, before[attributeName], after[attributeName])
				}
			}
		})
	}
}

// TestEveryMonitorTypeHandlesNullAttributes covers the other half of each hook:
// what happens when the user set none of the optional attributes. Sending a
// zero value where the server expects null is how NOT NULL constraint errors and
// spurious diffs appear.
func TestEveryMonitorTypeHandlesNullAttributes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	for name, res := range monitorResources(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			schemaResp := schemaOf(t, res)
			objectType, ok := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
			if !ok {
				t.Fatal("a resource schema is always an object")
			}

			model := res.NewModelForTest()
			plan := tfsdk.Plan{Schema: schemaResp.Schema, Raw: nullObject(t, objectType)}
			if diags := plan.Get(ctx, model); diags.HasError() {
				t.Fatalf("reading an all-null plan: %s", diags)
			}

			wire := newWireMonitor(res.DefForTest().WireType)
			applyDiags := diagnostics()
			model.ApplyTo(ctx, wire, applyDiags)
			if applyDiags.HasError() {
				t.Fatalf("ApplyTo should tolerate unset attributes: %s", applyDiags)
			}

			readBack := res.NewModelForTest()
			readDiags := diagnostics()
			readBack.ReadFrom(ctx, wire, readDiags)
			if readDiags.HasError() {
				t.Fatalf("ReadFrom should tolerate an empty payload: %s", readDiags)
			}
		})
	}
}

// TestEveryMonitorTypeDeclaresItsWireType guards the registry itself: two types
// sharing a WireType, or an empty one, would silently make one of them
// unusable.
func TestEveryMonitorTypeDeclaresItsWireType(t *testing.T) {
	t.Parallel()

	seen := map[string]string{}
	for name, res := range monitorResources(t) {
		def := res.DefForTest()

		if def.WireType == "" {
			t.Errorf("%s has no WireType", name)
		}
		if def.Description == "" {
			t.Errorf("%s has no description, so its docs page would be empty", name)
		}
		if !strings.HasPrefix(name, "uptimekuma_monitor") {
			t.Errorf("%s is not named as a monitor", name)
		}
		if previous, duplicate := seen[def.WireType]; duplicate {
			t.Errorf("%s and %s both claim wire type %q", previous, name, def.WireType)
		}
		seen[def.WireType] = name
	}

	if len(seen) != len(monitorResources(t)) {
		t.Errorf("wire types are not unique: %d types, %d wire types", len(monitorResources(t)), len(seen))
	}
}

func TestMonitorTypeCount(t *testing.T) {
	t.Parallel()

	// A reminder to update the docs and the demo when a type is added.
	const implemented = 9
	if got := len(monitorResources(t)); got != implemented {
		t.Errorf("%d monitor types implemented, expected %d — update the guide in templates/ and examples/demo/", got, implemented)
	}
}

func serialize(t *testing.T, schemaResp fwresource.SchemaResponse, model any) map[string]tftypes.Value {
	t.Helper()

	ctx := context.Background()
	objectType, ok := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatal("a resource schema is always an object")
	}

	state := tfsdk.State{Schema: schemaResp.Schema, Raw: nullObject(t, objectType)}
	if diags := state.Set(ctx, model); diags.HasError() {
		t.Fatalf("serializing the model: %s", diags)
	}

	var object map[string]tftypes.Value
	if err := state.Raw.As(&object); err != nil {
		t.Fatalf("unpacking the state: %s", err)
	}
	return object
}

func diagnostics() *diag.Diagnostics { return &diag.Diagnostics{} }

// newWireMonitor is the payload the base CRUD would hand to a hook: the type is
// already set, everything else is up to the model.
func newWireMonitor(wireType string) *kuma.Monitor {
	return &kuma.Monitor{Type: wireType}
}
