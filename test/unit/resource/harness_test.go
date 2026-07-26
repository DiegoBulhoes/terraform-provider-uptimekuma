package resource_test

import (
	"context"
	"strings"
	"testing"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// A small harness for driving a resource's CRUD methods directly, with a mocked
// client.
//
// The acceptance tests cover the happy paths, but they cannot reach the error
// branches: there is no way to ask a real server to fail on demand, and the
// branch that matters most — a Read finding the object gone, so the resource
// drops out of state — is exactly the one a passing acceptance test never takes.
// Calling the methods here is the only way to pin those down.

// resourceUnderTest is a resource wired to a mock client.
type resourceUnderTest struct {
	res    fwresource.Resource
	schema fwresource.SchemaResponse
}

// configure builds the resource and hands it the mocked client.
func configure(t *testing.T, factory func() fwresource.Resource, client common.KumaClient) resourceUnderTest {
	t.Helper()

	ctx := context.Background()
	res := factory()

	schemaResp := fwresource.SchemaResponse{}
	res.Schema(ctx, fwresource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema: %s", schemaResp.Diagnostics)
	}

	withConfigure, ok := res.(fwresource.ResourceWithConfigure)
	if !ok {
		t.Fatal("resource does not implement Configure")
	}
	configureResp := &fwresource.ConfigureResponse{}
	withConfigure.Configure(ctx, fwresource.ConfigureRequest{ProviderData: client}, configureResp)
	if configureResp.Diagnostics.HasError() {
		t.Fatalf("configure: %s", configureResp.Diagnostics)
	}

	return resourceUnderTest{res: res, schema: schemaResp}
}

// state builds a state object where every attribute is null, then sets the given
// ones. Starting from a fully-null object matters: the framework refuses to set
// an attribute on a null state.
func (r resourceUnderTest) state(t *testing.T, values map[string]tftypes.Value) tfsdk.State {
	t.Helper()

	ctx := context.Background()
	objectType, ok := r.schema.Schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatal("a resource schema should always be an object")
	}

	attributes := make(map[string]tftypes.Value, len(objectType.AttributeTypes))
	for name, attributeType := range objectType.AttributeTypes {
		if value, given := values[name]; given {
			attributes[name] = value
			continue
		}
		attributes[name] = tftypes.NewValue(attributeType, nil)
	}

	return tfsdk.State{
		Schema: r.schema.Schema,
		Raw:    tftypes.NewValue(objectType, attributes),
	}
}

// read calls Read and reports whether the resource removed itself from state.
func (r resourceUnderTest) read(t *testing.T, state tfsdk.State) (removed bool, diagnostics string) {
	t.Helper()

	resp := &fwresource.ReadResponse{State: state}
	r.res.Read(context.Background(), fwresource.ReadRequest{State: state}, resp)

	// RemoveResource sets the raw value to null, which is how the framework
	// signals "this is gone".
	return resp.State.Raw.IsNull(), renderErrors(resp.Diagnostics)
}

// delete calls Delete and returns the errors it produced.
func (r resourceUnderTest) delete(t *testing.T, state tfsdk.State) string {
	t.Helper()

	resp := &fwresource.DeleteResponse{State: state}
	r.res.Delete(context.Background(), fwresource.DeleteRequest{State: state}, resp)
	return renderErrors(resp.Diagnostics)
}

// create calls Create with the given plan and returns the errors it produced.
func (r resourceUnderTest) create(t *testing.T, plan tfsdk.State) string {
	t.Helper()

	resp := &fwresource.CreateResponse{State: plan}
	r.res.Create(context.Background(), fwresource.CreateRequest{
		Plan: tfsdk.Plan(plan),
	}, resp)
	return renderErrors(resp.Diagnostics)
}

// update calls Update and returns the errors it produced.
func (r resourceUnderTest) update(t *testing.T, plan, state tfsdk.State) string {
	t.Helper()

	resp := &fwresource.UpdateResponse{State: state}
	r.res.Update(context.Background(), fwresource.UpdateRequest{
		Plan:  tfsdk.Plan(plan),
		State: state,
	}, resp)
	return renderErrors(resp.Diagnostics)
}

// importState calls ImportState with the given ID and returns the errors.
func (r resourceUnderTest) importState(t *testing.T, id string) string {
	t.Helper()

	withImport, ok := r.res.(fwresource.ResourceWithImportState)
	if !ok {
		t.Fatal("resource does not implement ImportState")
	}

	resp := &fwresource.ImportStateResponse{
		State: r.state(t, nil),
	}
	withImport.ImportState(context.Background(), fwresource.ImportStateRequest{ID: id}, resp)
	return renderErrors(resp.Diagnostics)
}

// renderErrors turns the error diagnostics into one string, so tests can assert
// both that something failed and that the message says the right thing.
func renderErrors(diagnostics diag.Diagnostics) string {
	var parts []string
	for _, d := range diagnostics.Errors() {
		parts = append(parts, d.Summary()+": "+d.Detail())
	}
	return strings.Join(parts, "\n")
}

// str is shorthand for a known string attribute.
func str(value string) tftypes.Value {
	return tftypes.NewValue(tftypes.String, value)
}

// num is shorthand for a known number attribute.
func num(value int64) tftypes.Value {
	return tftypes.NewValue(tftypes.Number, value)
}

// boolean is shorthand for a known bool attribute.
func boolean(value bool) tftypes.Value {
	return tftypes.NewValue(tftypes.Bool, value)
}
