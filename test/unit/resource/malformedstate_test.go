package resource_test

import (
	"context"
	"testing"

	kumaresource "github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/test/mocks"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"go.uber.org/mock/gomock"
)

// State or plan that will not decode into the model.
//
// Every CRUD method starts the same way: read the plan or state into the model,
// then stop if that produced errors. The guard looks like boilerplate, and it is
// the kind of line that gets dropped when a resource is written by copying
// another one.
//
// What it prevents is specific. Without it the method continues with a zero-value
// model — empty ID, empty name — and goes on to call the server with it. Delete
// would ask the server to remove id 0. Create would write a nameless object. The
// operation reports success, and the state file records something that does not
// match anything on the server.
//
// This walks every registered resource and every operation, so the guarantee
// covers resources added later too. The malformed value stands in for anything
// that makes the decode fail: a state file written by a different provider
// version, a hand edit, a type changed in the schema without a state upgrader.

// mistypedObject returns a value with the schema's attribute names but one
// attribute's type swapped, which is what the framework cannot convert.
func mistypedObject(t *testing.T, schema fwresource.SchemaResponse) tftypes.Value {
	t.Helper()

	ctx := context.Background()
	objectType, ok := schema.Schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatal("a resource schema is always an object")
	}

	// id is present on every resource in this provider and is always a string, so
	// handing over a boolean is guaranteed to fail the conversion.
	if _, hasID := objectType.AttributeTypes["id"]; !hasID {
		t.Fatal("every resource is expected to have an id attribute")
	}

	mistyped := tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}
	attributes := map[string]tftypes.Value{}
	for name, attributeType := range objectType.AttributeTypes {
		if name == "id" {
			mistyped.AttributeTypes[name] = tftypes.Bool
			attributes[name] = tftypes.NewValue(tftypes.Bool, true)
			continue
		}
		mistyped.AttributeTypes[name] = attributeType
		attributes[name] = tftypes.NewValue(attributeType, nil)
	}
	return tftypes.NewValue(mistyped, attributes)
}

func TestEveryOperationStopsOnAnUndecodableModel(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	for _, factory := range kumaresource.All() {
		res := factory()

		metadataResp := &fwresource.MetadataResponse{}
		res.Metadata(ctx, fwresource.MetadataRequest{ProviderTypeName: "uptimekuma"}, metadataResp)

		t.Run(metadataResp.TypeName, func(t *testing.T) {
			t.Parallel()

			schemaResp := fwresource.SchemaResponse{}
			res.Schema(ctx, fwresource.SchemaRequest{}, &schemaResp)
			if schemaResp.Diagnostics.HasError() {
				t.Fatalf("schema: %s", schemaResp.Diagnostics)
			}

			raw := mistypedObject(t, schemaResp)

			// A controller with no expectations: if any operation reaches the client
			// despite the failed decode, GoMock fails the test. That is the real
			// assertion here.
			newResource := func(t *testing.T) fwresource.Resource {
				t.Helper()

				client := mocks.NewMockKumaClient(gomock.NewController(t))
				fresh := factory()
				withConfigure, ok := fresh.(fwresource.ResourceWithConfigure)
				if !ok {
					t.Fatal("every resource needs Configure")
				}
				configureResp := &fwresource.ConfigureResponse{}
				withConfigure.Configure(ctx, fwresource.ConfigureRequest{ProviderData: client}, configureResp)
				if configureResp.Diagnostics.HasError() {
					t.Fatalf("configure: %s", configureResp.Diagnostics)
				}
				return fresh
			}

			t.Run("Create", func(t *testing.T) {
				t.Parallel()

				resp := &fwresource.CreateResponse{
					State: tfsdk.State{Schema: schemaResp.Schema, Raw: raw},
				}
				newResource(t).Create(ctx, fwresource.CreateRequest{
					Plan: tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw},
				}, resp)

				if !resp.Diagnostics.HasError() {
					t.Error("Create continued with a plan it could not decode; it would " +
						"write a zero-value object to the server")
				}
			})

			t.Run("Read", func(t *testing.T) {
				t.Parallel()

				resp := &fwresource.ReadResponse{
					State: tfsdk.State{Schema: schemaResp.Schema, Raw: raw},
				}
				newResource(t).Read(ctx, fwresource.ReadRequest{
					State: tfsdk.State{Schema: schemaResp.Schema, Raw: raw},
				}, resp)

				if !resp.Diagnostics.HasError() {
					t.Error("Read continued with a state it could not decode")
				}
			})

			t.Run("Update", func(t *testing.T) {
				t.Parallel()

				resp := &fwresource.UpdateResponse{
					State: tfsdk.State{Schema: schemaResp.Schema, Raw: raw},
				}
				newResource(t).Update(ctx, fwresource.UpdateRequest{
					Plan:  tfsdk.Plan{Schema: schemaResp.Schema, Raw: raw},
					State: tfsdk.State{Schema: schemaResp.Schema, Raw: raw},
				}, resp)

				if !resp.Diagnostics.HasError() {
					t.Error("Update continued with a plan it could not decode")
				}
			})

			t.Run("Delete", func(t *testing.T) {
				t.Parallel()

				resp := &fwresource.DeleteResponse{
					State: tfsdk.State{Schema: schemaResp.Schema, Raw: raw},
				}
				newResource(t).Delete(ctx, fwresource.DeleteRequest{
					State: tfsdk.State{Schema: schemaResp.Schema, Raw: raw},
				}, resp)

				// Settings is the one exception, and deliberately so: it is a singleton
				// with nothing to delete — Uptime Kuma cannot remove a setting, and
				// reverting to some notion of "default" would be a guess. Its Delete
				// never reads the state, so an undecodable one changes nothing. It warns
				// instead, and that warning is the contract.
				if metadataResp.TypeName == "uptimekuma_settings" {
					if !hasWarning(resp) {
						t.Error("destroying the settings resource must warn that the values " +
							"stay in effect on the server")
					}
					return
				}

				if !resp.Diagnostics.HasError() {
					t.Error("Delete continued with a state it could not decode; it would " +
						"ask the server to remove id 0")
				}
			})
		})
	}
}

// hasWarning reports whether any diagnostic is a warning.
func hasWarning(resp *fwresource.DeleteResponse) bool {
	return len(resp.Diagnostics.Warnings()) > 0
}
