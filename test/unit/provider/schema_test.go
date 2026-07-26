package provider_test

import (
	"context"
	"strings"
	"testing"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	providerschema "github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

// TestProviderSchema checks the provider's own schema is implementable.
func TestProviderSchema(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	p := provider.New("test")()

	resp := &fwprovider.SchemaResponse{}
	p.Schema(ctx, fwprovider.SchemaRequest{}, resp)

	if resp.Diagnostics.HasError() {
		t.Fatalf("provider schema has errors: %s", resp.Diagnostics)
	}

	for _, name := range []string{"endpoint", "username", "password", "token", "timeout", "max_retries", "insecure_skip_verify"} {
		if _, ok := resp.Schema.Attributes[name]; !ok {
			t.Errorf("provider schema is missing attribute %q", name)
		}
	}

	// Credentials must never leak into logs or plan output.
	for _, name := range []string{"password", "token"} {
		attribute, ok := resp.Schema.Attributes[name].(providerschema.StringAttribute)
		if !ok {
			t.Errorf("attribute %q is not a string attribute", name)
			continue
		}
		if !attribute.Sensitive {
			t.Errorf("attribute %q must be marked sensitive", name)
		}
	}
}

// TestResourceSchemas validates every resource schema.
//
// ValidateImplementation is what catches the mistakes this provider is most
// exposed to: the per-type monitor models are assembled from embedded structs
// and a merged attribute map, so a missing or duplicated attribute is a real
// possibility and would otherwise only surface at apply time.
func TestResourceSchemas(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	p := provider.New("test")()

	resources := p.Resources(ctx)
	if len(resources) == 0 {
		t.Fatal("the provider registers no resources")
	}

	seen := make(map[string]struct{}, len(resources))
	for _, factory := range resources {
		r := factory()

		metadataResp := &resource.MetadataResponse{}
		r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "uptimekuma"}, metadataResp)
		name := metadataResp.TypeName

		if name == "" {
			t.Error("a resource reports an empty type name")
			continue
		}
		if !strings.HasPrefix(name, "uptimekuma_") {
			t.Errorf("resource %q does not use the provider prefix", name)
		}
		if _, duplicate := seen[name]; duplicate {
			t.Errorf("resource %q is registered twice", name)
		}
		seen[name] = struct{}{}

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			schemaResp := &resource.SchemaResponse{}
			r.Schema(ctx, resource.SchemaRequest{}, schemaResp)
			if schemaResp.Diagnostics.HasError() {
				t.Fatalf("schema has errors: %s", schemaResp.Diagnostics)
			}

			if diags := schemaResp.Schema.ValidateImplementation(ctx); diags.HasError() {
				t.Fatalf("schema is not implementable: %s", diags)
			}

			// Every resource needs an id to be addressable and importable.
			if _, ok := schemaResp.Schema.Attributes["id"]; !ok {
				t.Error("schema is missing the id attribute")
			}
		})
	}
}

// TestMonitorResourcesShareBaseAttributes ensures the shared monitor schema is
// really shared, so a config written against one monitor type carries over.
func TestMonitorResourcesShareBaseAttributes(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	p := provider.New("test")()

	baseAttributes := []string{
		"id", "name", "description", "active", "interval", "retry_interval",
		"resend_interval", "max_retries", "upside_down", "weight", "parent_id",
		"notification_ids", "tags",
	}

	monitorCount := 0
	for _, factory := range p.Resources(ctx) {
		r := factory()

		metadataResp := &resource.MetadataResponse{}
		r.Metadata(ctx, resource.MetadataRequest{ProviderTypeName: "uptimekuma"}, metadataResp)
		if !strings.HasPrefix(metadataResp.TypeName, "uptimekuma_monitor") {
			continue
		}
		monitorCount++

		schemaResp := &resource.SchemaResponse{}
		r.Schema(ctx, resource.SchemaRequest{}, schemaResp)

		for _, name := range baseAttributes {
			if _, ok := schemaResp.Schema.Attributes[name]; !ok {
				t.Errorf("%s is missing base attribute %q", metadataResp.TypeName, name)
			}
		}
	}

	if monitorCount == 0 {
		t.Fatal("no monitor resources are registered")
	}
}

// TestDataSourceSchemas validates every data source schema.
func TestDataSourceSchemas(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	p := provider.New("test")()

	for _, factory := range p.DataSources(ctx) {
		d := factory()

		metadataResp := &datasource.MetadataResponse{}
		d.Metadata(ctx, datasource.MetadataRequest{ProviderTypeName: "uptimekuma"}, metadataResp)
		name := metadataResp.TypeName

		t.Run(name, func(t *testing.T) {
			t.Parallel()

			schemaResp := &datasource.SchemaResponse{}
			d.Schema(ctx, datasource.SchemaRequest{}, schemaResp)
			if schemaResp.Diagnostics.HasError() {
				t.Fatalf("schema has errors: %s", schemaResp.Diagnostics)
			}

			if diags := schemaResp.Schema.ValidateImplementation(ctx); diags.HasError() {
				t.Fatalf("schema is not implementable: %s", diags)
			}
		})
	}
}
