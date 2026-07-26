package datasource_test

import (
	"context"
	"strings"
	"testing"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	kumadatasource "github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/datasource"
	apikeyds "github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/datasource/apikey"
	dockerhostds "github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/datasource/dockerhost"
	maintenanceds "github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/datasource/maintenance"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/datasource/monitor"
	notificationds "github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/datasource/notification"
	proxyds "github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/datasource/proxy"
	settingsds "github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/datasource/settings"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/datasource/statuspage"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/datasource/tag"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/test/mocks"
	fwdatasource "github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"go.uber.org/mock/gomock"
)

// The id-or-name lookups have three outcomes an acceptance test cannot easily
// produce: both given, neither given, and a name matching more than one object.
// Uptime Kuma allows duplicate tag names, so the last one is real.

func ptr[T any](v T) *T { return &v }

type dataSourceUnderTest struct {
	ds     fwdatasource.DataSource
	schema fwdatasource.SchemaResponse
}

func configure(t *testing.T, factory func() fwdatasource.DataSource, client common.KumaClient) dataSourceUnderTest {
	t.Helper()

	ctx := context.Background()
	ds := factory()

	schemaResp := fwdatasource.SchemaResponse{}
	ds.Schema(ctx, fwdatasource.SchemaRequest{}, &schemaResp)
	if schemaResp.Diagnostics.HasError() {
		t.Fatalf("schema: %s", schemaResp.Diagnostics)
	}

	withConfigure, ok := ds.(fwdatasource.DataSourceWithConfigure)
	if !ok {
		t.Fatal("data source does not implement Configure")
	}
	configureResp := &fwdatasource.ConfigureResponse{}
	withConfigure.Configure(ctx, fwdatasource.ConfigureRequest{ProviderData: client}, configureResp)
	if configureResp.Diagnostics.HasError() {
		t.Fatalf("configure: %s", configureResp.Diagnostics)
	}

	return dataSourceUnderTest{ds: ds, schema: schemaResp}
}

func (d dataSourceUnderTest) read(t *testing.T, values map[string]tftypes.Value) string {
	t.Helper()

	ctx := context.Background()
	objectType, ok := d.schema.Schema.Type().TerraformType(ctx).(tftypes.Object)
	if !ok {
		t.Fatal("a data source schema should always be an object")
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

	resp := &fwdatasource.ReadResponse{
		State: tfsdk.State{Schema: d.schema.Schema, Raw: raw},
	}
	d.ds.Read(ctx, fwdatasource.ReadRequest{
		Config: tfsdk.Config{Schema: d.schema.Schema, Raw: raw},
	}, resp)

	return renderErrors(resp.Diagnostics)
}

func renderErrors(diagnostics diag.Diagnostics) string {
	var parts []string
	for _, d := range diagnostics.Errors() {
		parts = append(parts, d.Summary()+": "+d.Detail())
	}
	return strings.Join(parts, "\n")
}

func str(value string) tftypes.Value { return tftypes.NewValue(tftypes.String, value) }

// ── Monitor ─────────────────────────────────────────────────────────

func TestMonitorLookup(t *testing.T) {
	t.Parallel()

	found := &kuma.Monitor{
		ID: 7, Name: "API", Type: "http", URL: ptr("https://api.example.com"),
		Interval: 60, Active: kuma.BoolPtr(true),
		Tags: []kuma.MonitorTag{
			{TagID: 1, Name: "env", Color: "#fff", Value: ptr("prod")},
			{TagID: 2, Name: "team", Color: "#000", Value: nil},
		},
	}

	t.Run("by id", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().GetMonitor(gomock.Any(), 7).Return(found, nil)

		d := configure(t, monitor.New, client)
		if errs := d.read(t, map[string]tftypes.Value{"id": str("7")}); errs != "" {
			t.Errorf("unexpected diagnostics: %s", errs)
		}
	})

	t.Run("by name", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().ListMonitors(gomock.Any()).Return(map[int]kuma.Monitor{
			7: *found,
			8: {ID: 8, Name: "Other", Type: "ping"},
		}, nil)

		d := configure(t, monitor.New, client)
		if errs := d.read(t, map[string]tftypes.Value{"name": str("API")}); errs != "" {
			t.Errorf("unexpected diagnostics: %s", errs)
		}
	})

	t.Run("both id and name is rejected", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		d := configure(t, monitor.New, client)

		errs := d.read(t, map[string]tftypes.Value{"id": str("7"), "name": str("API")})
		if errs == "" {
			t.Fatal("expected a diagnostic")
		}
		if !strings.Contains(errs, "Ambiguous") {
			t.Errorf("the message should say the lookup is ambiguous: %s", errs)
		}
	})

	t.Run("neither id nor name is rejected", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		d := configure(t, monitor.New, client)

		errs := d.read(t, nil)
		if errs == "" {
			t.Fatal("expected a diagnostic")
		}
		if !strings.Contains(errs, "Missing") {
			t.Errorf("the message should say what is missing: %s", errs)
		}
	})

	t.Run("a name matching nothing is rejected", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().ListMonitors(gomock.Any()).Return(map[int]kuma.Monitor{}, nil)

		d := configure(t, monitor.New, client)
		if errs := d.read(t, map[string]tftypes.Value{"name": str("nope")}); errs == "" {
			t.Error("expected a diagnostic")
		}
	})

	t.Run("a name matching several is rejected", func(t *testing.T) {
		t.Parallel()

		// Monitor names are not unique, so picking one silently is a coin toss.
		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().ListMonitors(gomock.Any()).Return(map[int]kuma.Monitor{
			7: {ID: 7, Name: "API", Type: "http"},
			8: {ID: 8, Name: "API", Type: "ping"},
		}, nil)

		d := configure(t, monitor.New, client)
		errs := d.read(t, map[string]tftypes.Value{"name": str("API")})
		if errs == "" {
			t.Fatal("expected a diagnostic")
		}
		if !strings.Contains(errs, "id") {
			t.Errorf("the message should suggest looking up by id: %s", errs)
		}
	})

	t.Run("a failed list surfaces", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().ListMonitors(gomock.Any()).Return(nil, kuma.ErrTimeout)

		d := configure(t, monitor.New, client)
		if errs := d.read(t, map[string]tftypes.Value{"name": str("API")}); errs == "" {
			t.Error("expected a diagnostic")
		}
	})

	t.Run("a non-numeric id is rejected", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		d := configure(t, monitor.New, client)

		if errs := d.read(t, map[string]tftypes.Value{"id": str("abc")}); errs == "" {
			t.Error("expected a diagnostic")
		}
	})
}

func TestMonitorList(t *testing.T) {
	t.Parallel()

	all := map[int]kuma.Monitor{
		7: {ID: 7, Name: "API", Type: "http", URL: ptr("https://a"), Interval: 60},
		8: {ID: 8, Name: "Ping", Type: "ping", Hostname: ptr("127.0.0.1"), Interval: 60},
		9: {ID: 9, Name: "Web", Type: "http", URL: ptr("https://b"), Interval: 60, Parent: ptr(7)},
	}

	t.Run("unfiltered", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().ListMonitors(gomock.Any()).Return(all, nil)

		d := configure(t, monitor.NewList, client)
		if errs := d.read(t, nil); errs != "" {
			t.Errorf("unexpected diagnostics: %s", errs)
		}
	})

	t.Run("filtered by type", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().ListMonitors(gomock.Any()).Return(all, nil)

		d := configure(t, monitor.NewList, client)
		if errs := d.read(t, map[string]tftypes.Value{"type": str("http")}); errs != "" {
			t.Errorf("unexpected diagnostics: %s", errs)
		}
	})

	t.Run("a failed list surfaces", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().ListMonitors(gomock.Any()).Return(nil, kuma.ErrTimeout)

		d := configure(t, monitor.NewList, client)
		if errs := d.read(t, nil); errs == "" {
			t.Error("expected a diagnostic")
		}
	})
}

// ── Tag ─────────────────────────────────────────────────────────────

func TestTagLookup(t *testing.T) {
	t.Parallel()

	tags := []kuma.Tag{
		{ID: 1, Name: "env", Color: "#4B5563"},
		{ID: 2, Name: "team", Color: "#059669"},
	}

	t.Run("by id", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().ListTags(gomock.Any()).Return(tags, nil)

		d := configure(t, tag.New, client)
		if errs := d.read(t, map[string]tftypes.Value{"id": str("2")}); errs != "" {
			t.Errorf("unexpected diagnostics: %s", errs)
		}
	})

	t.Run("by name", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().ListTags(gomock.Any()).Return(tags, nil)

		d := configure(t, tag.New, client)
		if errs := d.read(t, map[string]tftypes.Value{"name": str("env")}); errs != "" {
			t.Errorf("unexpected diagnostics: %s", errs)
		}
	})

	t.Run("an id that does not exist is rejected", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().ListTags(gomock.Any()).Return(tags, nil)

		d := configure(t, tag.New, client)
		if errs := d.read(t, map[string]tftypes.Value{"id": str("99")}); errs == "" {
			t.Error("expected a diagnostic")
		}
	})

	t.Run("a name that does not exist is rejected", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().ListTags(gomock.Any()).Return(tags, nil)

		d := configure(t, tag.New, client)
		if errs := d.read(t, map[string]tftypes.Value{"name": str("nope")}); errs == "" {
			t.Error("expected a diagnostic")
		}
	})

	t.Run("a duplicate name is rejected", func(t *testing.T) {
		t.Parallel()

		// Uptime Kuma really does allow two tags with the same name.
		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().ListTags(gomock.Any()).Return([]kuma.Tag{
			{ID: 1, Name: "env", Color: "#a"},
			{ID: 2, Name: "env", Color: "#b"},
		}, nil)

		d := configure(t, tag.New, client)
		errs := d.read(t, map[string]tftypes.Value{"name": str("env")})
		if errs == "" {
			t.Fatal("expected a diagnostic")
		}
		if !strings.Contains(errs, "Ambiguous") {
			t.Errorf("the message should say the name is ambiguous: %s", errs)
		}
	})

	t.Run("both id and name is rejected", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		d := configure(t, tag.New, client)

		if errs := d.read(t, map[string]tftypes.Value{"id": str("1"), "name": str("env")}); errs == "" {
			t.Error("expected a diagnostic")
		}
	})

	t.Run("neither id nor name is rejected", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		d := configure(t, tag.New, client)

		if errs := d.read(t, nil); errs == "" {
			t.Error("expected a diagnostic")
		}
	})

	t.Run("a failed list surfaces", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().ListTags(gomock.Any()).Return(nil, kuma.ErrTimeout)

		d := configure(t, tag.New, client)
		if errs := d.read(t, map[string]tftypes.Value{"name": str("env")}); errs == "" {
			t.Error("expected a diagnostic")
		}
	})

	t.Run("a non-numeric id is rejected", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().ListTags(gomock.Any()).Return(tags, nil)

		d := configure(t, tag.New, client)
		if errs := d.read(t, map[string]tftypes.Value{"id": str("abc")}); errs == "" {
			t.Error("expected a diagnostic")
		}
	})
}

func TestTagList(t *testing.T) {
	t.Parallel()

	t.Run("returns every tag", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().ListTags(gomock.Any()).Return([]kuma.Tag{
			{ID: 2, Name: "team", Color: "#b"},
			{ID: 1, Name: "env", Color: "#a"},
		}, nil)

		d := configure(t, tag.NewList, client)
		if errs := d.read(t, nil); errs != "" {
			t.Errorf("unexpected diagnostics: %s", errs)
		}
	})

	t.Run("a failure surfaces", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().ListTags(gomock.Any()).Return(nil, kuma.ErrTimeout)

		d := configure(t, tag.NewList, client)
		if errs := d.read(t, nil); errs == "" {
			t.Error("expected a diagnostic")
		}
	})
}

// ── Status page ─────────────────────────────────────────────────────

func TestStatusPageDataSource(t *testing.T) {
	t.Parallel()

	t.Run("reads the configuration and the group tree", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().GetStatusPage(gomock.Any(), "public").Return(&kuma.StatusPage{
			ID: 1, Slug: "public", Title: "Status", Description: ptr("d"), Theme: "dark",
			Published: kuma.BoolPtr(true), ShowTags: kuma.BoolPtr(true),
			DomainNameList: []string{"status.example.com"},
		}, nil)
		client.EXPECT().GetStatusPageGroups(gomock.Any(), "public").Return([]kuma.StatusPageGroup{
			{ID: 1, Name: "Core", MonitorList: []kuma.StatusPageMonitor{{ID: 10}, {ID: 11}}},
			{ID: 2, Name: "Empty"},
		}, nil)

		d := configure(t, statuspage.New, client)
		if errs := d.read(t, map[string]tftypes.Value{"slug": str("public")}); errs != "" {
			t.Errorf("unexpected diagnostics: %s", errs)
		}
	})

	t.Run("a missing page surfaces", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().GetStatusPage(gomock.Any(), "nope").Return(nil, kuma.ErrNotFound)

		d := configure(t, statuspage.New, client)
		if errs := d.read(t, map[string]tftypes.Value{"slug": str("nope")}); errs == "" {
			t.Error("expected a diagnostic")
		}
	})

	t.Run("a failing group read surfaces", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().GetStatusPage(gomock.Any(), "public").
			Return(&kuma.StatusPage{ID: 1, Slug: "public", Title: "T", Theme: "auto"}, nil)
		client.EXPECT().GetStatusPageGroups(gomock.Any(), "public").Return(nil, kuma.ErrTimeout)

		d := configure(t, statuspage.New, client)
		if errs := d.read(t, map[string]tftypes.Value{"slug": str("public")}); errs == "" {
			t.Error("expected a diagnostic")
		}
	})
}

func TestStatusPageList(t *testing.T) {
	t.Parallel()

	t.Run("reconnects to get a current list", func(t *testing.T) {
		t.Parallel()

		// refresh=true: the list only arrives at login, so a page created earlier in
		// the same run would be missing.
		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().ListStatusPages(gomock.Any(), true).Return(map[int]kuma.StatusPage{
			2: {ID: 2, Slug: "b", Title: "B", Published: kuma.BoolPtr(false)},
			1: {ID: 1, Slug: "a", Title: "A", Published: kuma.BoolPtr(true)},
		}, nil)

		d := configure(t, statuspage.NewList, client)
		if errs := d.read(t, nil); errs != "" {
			t.Errorf("unexpected diagnostics: %s", errs)
		}
	})

	t.Run("a failure surfaces", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().ListStatusPages(gomock.Any(), true).Return(nil, kuma.ErrTimeout)

		d := configure(t, statuspage.NewList, client)
		if errs := d.read(t, nil); errs == "" {
			t.Error("expected a diagnostic")
		}
	})
}

// TestEveryListDataSourceHandlesFailure walks the remaining list data sources,
// which are all the same shape: one client call, sorted output.
func TestEveryListDataSourceHandlesFailure(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	names := map[string]bool{}
	for _, factory := range kumadatasource.All() {
		ds := factory()
		metadataResp := &fwdatasource.MetadataResponse{}
		ds.Metadata(ctx, fwdatasource.MetadataRequest{ProviderTypeName: "uptimekuma"}, metadataResp)
		names[metadataResp.TypeName] = true
	}

	// Fails if a data source is added without a test here.
	for _, expected := range []string{
		"uptimekuma_monitor", "uptimekuma_monitors", "uptimekuma_tag", "uptimekuma_tags",
		"uptimekuma_notifications", "uptimekuma_maintenances", "uptimekuma_status_page",
		"uptimekuma_status_pages", "uptimekuma_proxies", "uptimekuma_docker_hosts",
		"uptimekuma_api_keys", "uptimekuma_settings", "uptimekuma_info",
	} {
		if !names[expected] {
			t.Errorf("%s is no longer registered", expected)
		}
	}
	if len(names) != 13 {
		t.Errorf("found %d data sources; add coverage for the new one", len(names))
	}
}

// A data source that continues with a zero-value model looks up the empty string
// or id 0, and whatever it finds has nothing to do with the request.
func TestEveryDataSourceStopsOnAnUndecodableConfig(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	for _, factory := range kumadatasource.All() {
		ds := factory()

		metadataResp := &fwdatasource.MetadataResponse{}
		ds.Metadata(ctx, fwdatasource.MetadataRequest{ProviderTypeName: "uptimekuma"}, metadataResp)

		t.Run(metadataResp.TypeName, func(t *testing.T) {
			t.Parallel()

			schemaResp := fwdatasource.SchemaResponse{}
			ds.Schema(ctx, fwdatasource.SchemaRequest{}, &schemaResp)
			if schemaResp.Diagnostics.HasError() {
				t.Fatalf("schema: %s", schemaResp.Diagnostics)
			}

			objectType, ok := schemaResp.Schema.Type().TerraformType(ctx).(tftypes.Object)
			if !ok {
				t.Fatal("a data source schema is always an object")
			}

			// No common attribute here the way resources all have id, so the first
			// string in the schema stands in.
			var target string
			for name, attributeType := range objectType.AttributeTypes {
				if attributeType.Is(tftypes.String) {
					target = name
					break
				}
			}
			if target == "" {
				t.Skip("no string attribute to mistype")
			}

			mistyped := tftypes.Object{AttributeTypes: map[string]tftypes.Type{}}
			attributes := map[string]tftypes.Value{}
			for name, attributeType := range objectType.AttributeTypes {
				if name == target {
					mistyped.AttributeTypes[name] = tftypes.Bool
					attributes[name] = tftypes.NewValue(tftypes.Bool, true)
					continue
				}
				mistyped.AttributeTypes[name] = attributeType
				attributes[name] = tftypes.NewValue(attributeType, nil)
			}
			raw := tftypes.NewValue(mistyped, attributes)

			client := mocks.NewMockKumaClient(gomock.NewController(t))
			// The two singletons take no input, so they fetch before writing state.
			switch metadataResp.TypeName {
			case "uptimekuma_info":
				client.EXPECT().Info().Return(kuma.ServerInfo{Version: "2.4.0"}).AnyTimes()
			case "uptimekuma_settings":
				client.EXPECT().GetSettings(gomock.Any()).Return(map[string]any{}, nil).AnyTimes()
			}

			fresh := factory()
			withConfigure, ok := fresh.(fwdatasource.DataSourceWithConfigure)
			if !ok {
				t.Fatal("every data source needs Configure")
			}
			configureResp := &fwdatasource.ConfigureResponse{}
			withConfigure.Configure(ctx, fwdatasource.ConfigureRequest{ProviderData: client}, configureResp)
			if configureResp.Diagnostics.HasError() {
				t.Fatalf("configure: %s", configureResp.Diagnostics)
			}

			resp := &fwdatasource.ReadResponse{
				State: tfsdk.State{Schema: schemaResp.Schema, Raw: raw},
			}
			fresh.Read(ctx, fwdatasource.ReadRequest{
				Config: tfsdk.Config{Schema: schemaResp.Schema, Raw: raw},
			}, resp)

			switch metadataResp.TypeName {
			case "uptimekuma_info", "uptimekuma_settings":
				// No config to decode. Adding an input attribute to either would need
				// the guard, and this test would then fail and say so.
				if resp.Diagnostics.HasError() {
					t.Errorf("%s takes no input, so a malformed config should not affect "+
						"it: %s", metadataResp.TypeName, resp.Diagnostics)
				}
			default:
				if !resp.Diagnostics.HasError() {
					t.Errorf("%s continued with a config it could not decode; the lookup "+
						"would be for an empty value", metadataResp.TypeName)
				}
			}
		})
	}
}

// An empty list instead of the failure is the damaging outcome: a for_each over
// uptimekuma_notifications would read it as "all deleted" and plan a destroy.
func TestEveryListDataSourceReportsAFailedFetch(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		factory func() fwdatasource.DataSource
		expect  func(*mocks.MockKumaClient)
	}{
		"notifications": {
			factory: notificationds.NewList,
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().ListNotifications(gomock.Any()).Return(nil, kuma.ErrTimeout)
			},
		},
		"proxies": {
			factory: proxyds.NewList,
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().ListProxies(gomock.Any()).Return(nil, kuma.ErrTimeout)
			},
		},
		"docker hosts": {
			factory: dockerhostds.NewList,
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().ListDockerHosts(gomock.Any()).Return(nil, kuma.ErrTimeout)
			},
		},
		"api keys": {
			factory: apikeyds.NewList,
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().ListAPIKeys(gomock.Any()).Return(nil, kuma.ErrTimeout)
			},
		},
		"maintenances": {
			factory: maintenanceds.NewList,
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().ListMaintenances(gomock.Any()).Return(nil, kuma.ErrTimeout)
			},
		},
		"settings": {
			factory: settingsds.New,
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().GetSettings(gomock.Any()).Return(nil, kuma.ErrTimeout)
			},
		},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client := mocks.NewMockKumaClient(gomock.NewController(t))
			tt.expect(client)

			d := configure(t, tt.factory, client)
			if errs := d.read(t, nil); errs == "" {
				t.Error("a failed fetch must surface: an empty list would look like " +
					"everything was deleted")
			}
		})
	}
}

// An instance with nothing configured yet is not an error.
func TestListDataSourcesSucceedOnAnEmptyServer(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		factory func() fwdatasource.DataSource
		expect  func(*mocks.MockKumaClient)
	}{
		"notifications": {
			factory: notificationds.NewList,
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().ListNotifications(gomock.Any()).Return(map[int]kuma.Notification{}, nil)
			},
		},
		"proxies": {
			factory: proxyds.NewList,
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().ListProxies(gomock.Any()).Return(map[int]kuma.Proxy{}, nil)
			},
		},
		"docker hosts": {
			factory: dockerhostds.NewList,
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().ListDockerHosts(gomock.Any()).Return(map[int]kuma.DockerHost{}, nil)
			},
		},
		"api keys": {
			factory: apikeyds.NewList,
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().ListAPIKeys(gomock.Any()).Return(map[int]kuma.APIKey{}, nil)
			},
		},
		"maintenances": {
			factory: maintenanceds.NewList,
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().ListMaintenances(gomock.Any()).Return(map[int]kuma.Maintenance{}, nil)
			},
		},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client := mocks.NewMockKumaClient(gomock.NewController(t))
			tt.expect(client)

			d := configure(t, tt.factory, client)
			if errs := d.read(t, nil); errs != "" {
				t.Errorf("an empty server is not an error: %s", errs)
			}
		})
	}
}
