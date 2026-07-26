package provider_test

import (
	"context"
	"strings"
	"testing"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// Every resource and data source starts with the same two guards in Configure:
// return quietly when provider data is absent, and complain when it is the wrong
// type. Both are easy to get wrong when adding a resource — forgetting the nil
// check makes `terraform validate` panic — and neither is reachable from an
// acceptance test, because the framework always hands over a real client.
//
// Walking the registry means a resource added later is covered without anyone
// remembering to write this test again.

func TestResourceConfigureHandlesAbsentProviderData(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	p := provider.New("test")()

	for _, factory := range p.Resources(ctx) {
		res := factory()

		metadataResp := &resource.MetadataResponse{}
		res.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "uptimekuma"}, metadataResp)

		t.Run(metadataResp.TypeName, func(t *testing.T) {
			t.Parallel()

			withConfigure, ok := res.(resource.ResourceWithConfigure)
			if !ok {
				t.Fatal("every resource needs Configure to receive the client")
			}

			// Terraform calls Configure with nil data during validate, before the
			// provider has been configured. Treating that as an error would break
			// `terraform validate` for everyone.
			resp := &resource.ConfigureResponse{}
			withConfigure.Configure(ctx, resource.ConfigureRequest{ProviderData: nil}, resp)
			if resp.Diagnostics.HasError() {
				t.Errorf("absent provider data must be ignored, got: %s", resp.Diagnostics)
			}
		})
	}
}

func TestResourceConfigureRejectsWrongProviderData(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	p := provider.New("test")()

	for _, factory := range p.Resources(ctx) {
		res := factory()

		metadataResp := &resource.MetadataResponse{}
		res.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "uptimekuma"}, metadataResp)

		t.Run(metadataResp.TypeName, func(t *testing.T) {
			t.Parallel()

			withConfigure := res.(resource.ResourceWithConfigure)

			// A wrong type means the provider handed over something unexpected,
			// which is a bug worth reporting rather than a nil client to panic on
			// later.
			resp := &resource.ConfigureResponse{}
			withConfigure.Configure(ctx, resource.ConfigureRequest{ProviderData: "not a client"}, resp)
			if !resp.Diagnostics.HasError() {
				t.Error("the wrong provider data type should be reported")
			}
			if got := resp.Diagnostics.Errors()[0].Detail(); !strings.Contains(got, "KumaClient") {
				t.Errorf("the message should name the expected type: %s", got)
			}
		})
	}
}

func TestDataSourceConfigureHandlesProviderData(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	p := provider.New("test")()

	for _, factory := range p.DataSources(ctx) {
		ds := factory()

		metadataResp := &datasource.MetadataResponse{}
		ds.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "uptimekuma"}, metadataResp)

		t.Run(metadataResp.TypeName, func(t *testing.T) {
			t.Parallel()

			withConfigure, ok := ds.(datasource.DataSourceWithConfigure)
			if !ok {
				t.Fatal("every data source needs Configure to receive the client")
			}

			absent := &datasource.ConfigureResponse{}
			withConfigure.Configure(ctx, datasource.ConfigureRequest{ProviderData: nil}, absent)
			if absent.Diagnostics.HasError() {
				t.Errorf("absent provider data must be ignored, got: %s", absent.Diagnostics)
			}

			wrong := &datasource.ConfigureResponse{}
			withConfigure.Configure(ctx, datasource.ConfigureRequest{ProviderData: 42}, wrong)
			if !wrong.Diagnostics.HasError() {
				t.Error("the wrong provider data type should be reported")
			}
		})
	}
}

// TestProviderMetadata pins down the type name, which is the prefix of every
// resource in a user's configuration: changing it silently would break every
// existing state file.
func TestProviderMetadata(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	p := provider.New("1.2.3")()

	resp := &fwprovider.MetadataResponse{}
	p.Metadata(ctx, fwprovider.MetadataRequest{}, resp)

	if resp.TypeName != "uptimekuma" {
		t.Errorf("type name = %q, want uptimekuma", resp.TypeName)
	}
	if resp.Version != "1.2.3" {
		t.Errorf("version = %q, want the one passed to New", resp.Version)
	}
}
