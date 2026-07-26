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

// Every CRUD method reads the plan or state into the model, then stops on error.
// Without that guard the method continues with a zero-value model and calls the
// server with it: Delete removes id 0, Create writes a nameless object.

// mistypedObject swaps one attribute's type, which the framework cannot convert.
func mistypedObject(t *testing.T, schema fwresource.SchemaResponse) tftypes.Value {
	t.Helper()

	ctx := context.Background()
	objectType, ok := schema.Schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatal("a resource schema is always an object")
	}

	// id is always a string here, so a boolean is guaranteed to fail.
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

			// No expectations: GoMock fails if any operation reaches the client.
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

				// Settings is a singleton with nothing to delete, so its Delete never
				// reads the state. It warns instead, and that warning is the contract.
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

func hasWarning(resp *fwresource.DeleteResponse) bool {
	return len(resp.Diagnostics.Warnings()) > 0
}
