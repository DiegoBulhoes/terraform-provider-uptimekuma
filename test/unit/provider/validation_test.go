package provider_test

import (
	"context"
	"strings"
	"testing"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/provider"
	fwprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// The first errors a new user hits. Someone who found the API key setting in the
// UI needs the diagnostic to say it does not apply to this API.
//
// Every attribute falls back to an environment variable, so these clear them —
// otherwise the tests pass locally and fail in CI.

func configureWith(t *testing.T, values map[string]tftypes.Value) fwprovider.ConfigureResponse {
	t.Helper()

	// Not parallel: t.Setenv forbids it.
	for _, name := range []string{
		"UPTIME_KUMA_URL", "UPTIME_KUMA_USERNAME", "UPTIME_KUMA_PASSWORD", "UPTIME_KUMA_TOKEN",
	} {
		t.Setenv(name, "")
	}

	ctx := context.Background()
	p := provider.New("test")()

	schemaResp := &fwprovider.SchemaResponse{}
	p.Schema(ctx, fwprovider.SchemaRequest{}, schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema: %s", schemaResp.Diagnostics)
	}

	objectType, ok := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatal("the provider schema is always an object")
	}

	attributes := make(map[string]tftypes.Value, len(objectType.AttributeTypes))
	for name, attributeType := range objectType.AttributeTypes {
		if value, given := values[name]; given {
			attributes[name] = value
			continue
		}
		attributes[name] = tftypes.NewValue(attributeType, nil)
	}
	raw := tftypes.NewValue(objectType, attributes)

	resp := fwprovider.ConfigureResponse{}
	p.Configure(ctx, fwprovider.ConfigureRequest{
		Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
	}, &resp)
	return resp
}

func text(value string) tftypes.Value { return tftypes.NewValue(tftypes.String, value) }

func errorText(resp fwprovider.ConfigureResponse) string {
	var parts []string
	for _, d := range resp.Diagnostics.Errors() {
		parts = append(parts, d.Summary()+": "+d.Detail())
	}
	return strings.Join(parts, "\n")
}

func TestAMissingEndpointIsReported(t *testing.T) {
	resp := configureWith(t, map[string]tftypes.Value{
		"username": text("admin"),
		"password": text("secret"),
	})

	errs := errorText(resp)
	if errs == "" {
		t.Fatal("expected a diagnostic")
	}
	if !strings.Contains(errs, "endpoint") {
		t.Errorf("the message should name the attribute: %s", errs)
	}
	if !strings.Contains(errs, "UPTIME_KUMA_URL") {
		t.Errorf("the message should mention the environment variable too: %s", errs)
	}
}

func TestMissingCredentialsAreReported(t *testing.T) {
	cases := map[string]map[string]tftypes.Value{
		"neither": {
			"endpoint": text("http://127.0.0.1:1"),
		},
		"no password": {
			"endpoint": text("http://127.0.0.1:1"),
			"username": text("admin"),
		},
		"no username": {
			"endpoint": text("http://127.0.0.1:1"),
			"password": text("secret"),
		},
	}

	for name, values := range cases {
		t.Run(name, func(t *testing.T) {
			resp := configureWith(t, values)

			errs := errorText(resp)
			if errs == "" {
				t.Fatal("expected a diagnostic")
			}
			if !strings.Contains(errs, "username") || !strings.Contains(errs, "password") {
				t.Errorf("the message should name both attributes: %s", errs)
			}
			if !strings.Contains(errs, "API-key") {
				t.Errorf("the message should explain that API-key authentication does not "+
					"apply to this API, or a user who found that setting in the UI will "+
					"keep trying it: %s", errs)
			}
		})
	}
}

// Both are known before anything is contacted, so report them together.
func TestEverythingMissingIsReportedAtOnce(t *testing.T) {
	resp := configureWith(t, nil)

	if count := len(resp.Diagnostics.Errors()); count < 2 {
		t.Errorf("got %d errors, want both the endpoint and the credentials reported "+
			"together: %s", count, errorText(resp))
	}
}

// Where a wrong URL surfaces. It has to be a diagnostic, not a crash.
func TestAnUnreachableEndpointIsReportedNotPanicked(t *testing.T) {
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("configuring against an unreachable server panicked: %v", recovered)
		}
	}()

	resp := configureWith(t, map[string]tftypes.Value{
		"endpoint":    text("http://127.0.0.1:1"), // closed port
		"username":    text("admin"),
		"password":    text("secret"),
		"max_retries": tftypes.NewValue(tftypes.Number, 0),
		"timeout":     tftypes.NewValue(tftypes.Number, 1),
	})

	errs := errorText(resp)
	if errs == "" {
		t.Fatal("expected a connection diagnostic")
	}
	if !strings.Contains(errs, "connect") {
		t.Errorf("the message should say it could not connect: %s", errs)
	}
	if resp.ResourceData != nil || resp.DataSourceData != nil {
		t.Error("no client should be handed to the resources when the connection failed")
	}
}

// The fallback path, which is how the acceptance tests configure the provider.
func TestEnvironmentVariablesSupplyTheConfiguration(t *testing.T) {
	t.Setenv("UPTIME_KUMA_URL", "http://127.0.0.1:1")
	t.Setenv("UPTIME_KUMA_USERNAME", "admin")
	t.Setenv("UPTIME_KUMA_PASSWORD", "secret")

	ctx := context.Background()
	p := provider.New("test")()

	schemaResp := &fwprovider.SchemaResponse{}
	p.Schema(ctx, fwprovider.SchemaRequest{}, schemaResp)

	objectType, ok := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatal("the provider schema is always an object")
	}
	attributes := make(map[string]tftypes.Value, len(objectType.AttributeTypes))
	for name, attributeType := range objectType.AttributeTypes {
		switch name {
		case "max_retries":
			attributes[name] = tftypes.NewValue(tftypes.Number, 0)
		case "timeout":
			attributes[name] = tftypes.NewValue(tftypes.Number, 1)
		default:
			attributes[name] = tftypes.NewValue(attributeType, nil)
		}
	}

	resp := fwprovider.ConfigureResponse{}
	p.Configure(ctx, fwprovider.ConfigureRequest{
		Config: tfsdk.Config{
			Schema: schemaResp.Schema,
			Raw:    tftypes.NewValue(objectType, attributes),
		},
	}, &resp)

	// The connection fails, but getting that far means the variables were read.
	errs := errorText(resp)
	if errs == "" {
		t.Fatal("expected the connection to be attempted and to fail")
	}
	if strings.Contains(errs, "Missing") {
		t.Errorf("the environment variables were not picked up: %s", errs)
	}
}
