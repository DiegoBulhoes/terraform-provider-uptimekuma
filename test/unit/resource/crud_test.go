package resource_test

import (
	"context"
	"strings"
	"testing"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/apikey"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/dockerhost"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/maintenance"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/monitor"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/notification"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/proxy"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/remotebrowser"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/settings"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/statuspage"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/statuspageincident"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/tag"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/test/mocks"
	fwresource "github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"go.uber.org/mock/gomock"
)

// gone is the error the server produces for a missing row: a getter dereferences
// a null bean and the JavaScript TypeError comes back as the message.
var gone = &kuma.APIError{Event: "get", Msg: "Cannot read properties of null (reading 'id')"}

// denied is a rejection that must surface rather than being swallowed.
var denied = &kuma.APIError{Event: "any", Msg: "Permission denied."}

// TestReadDropsDeletedResources is the branch acceptance tests cannot reach: an
// object deleted outside Terraform must disappear from state instead of failing
// the run.
func TestReadDropsDeletedResources(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		factory func() fwresource.Resource
		state   map[string]tftypes.Value
		expect  func(*mocks.MockKumaClient)
	}{
		{
			name:    "monitor",
			factory: monitor.NewHTTPResource,
			state:   map[string]tftypes.Value{"id": str("7")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().GetMonitor(gomock.Any(), 7).Return(nil, gone)
			},
		},
		{
			name:    "tag",
			factory: tag.New,
			state:   map[string]tftypes.Value{"id": str("3")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().GetTag(gomock.Any(), 3).Return(nil, kuma.ErrNotFound)
			},
		},
		{
			name:    "notification",
			factory: notification.New,
			state:   map[string]tftypes.Value{"id": str("5")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().GetNotification(gomock.Any(), 5).Return(nil, kuma.ErrNotFound)
			},
		},
		{
			name:    "proxy",
			factory: proxy.New,
			state:   map[string]tftypes.Value{"id": str("2")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().GetProxy(gomock.Any(), 2).Return(nil, kuma.ErrNotFound)
			},
		},
		{
			name:    "docker host",
			factory: dockerhost.New,
			state:   map[string]tftypes.Value{"id": str("1")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().GetDockerHost(gomock.Any(), 1).Return(nil, kuma.ErrNotFound)
			},
		},
		{
			name:    "remote browser",
			factory: remotebrowser.New,
			state:   map[string]tftypes.Value{"id": str("1")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().GetRemoteBrowser(gomock.Any(), 1).Return(nil, kuma.ErrNotFound)
			},
		},
		{
			name:    "api key",
			factory: apikey.New,
			state:   map[string]tftypes.Value{"id": str("4")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().GetAPIKey(gomock.Any(), 4).Return(nil, kuma.ErrNotFound)
			},
		},
		{
			name:    "maintenance",
			factory: maintenance.New,
			state:   map[string]tftypes.Value{"id": str("6")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().GetMaintenance(gomock.Any(), 6).Return(nil, gone)
			},
		},
		{
			name:    "status page",
			factory: statuspage.New,
			state:   map[string]tftypes.Value{"id": str("public")},
			expect: func(c *mocks.MockKumaClient) {
				// Status pages are addressed by slug, not by number.
				c.EXPECT().GetStatusPage(gomock.Any(), "public").Return(nil, gone)
			},
		},
		{
			name:    "status page incident",
			factory: statuspageincident.New,
			state:   map[string]tftypes.Value{"id": str("public/9")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().GetIncidentHistory(gomock.Any(), "public").Return(nil, gone)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := mocks.NewMockKumaClient(gomock.NewController(t))
			tt.expect(client)

			r := configure(t, tt.factory, client)
			removed, errs := r.read(t, r.state(t, tt.state))

			if errs != "" {
				t.Fatalf("a missing object is not an error: %s", errs)
			}
			if !removed {
				t.Error("the resource should have been removed from state")
			}
		})
	}
}

// TestReadSurfacesRealFailures is the other half: a failure that is not
// not-found has to reach the user, and must not silently drop the resource.
func TestReadSurfacesRealFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		factory func() fwresource.Resource
		state   map[string]tftypes.Value
		expect  func(*mocks.MockKumaClient)
	}{
		{
			name:    "monitor",
			factory: monitor.NewPingResource,
			state:   map[string]tftypes.Value{"id": str("7")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().GetMonitor(gomock.Any(), 7).Return(nil, denied)
			},
		},
		{
			name:    "tag",
			factory: tag.New,
			state:   map[string]tftypes.Value{"id": str("3")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().GetTag(gomock.Any(), 3).Return(nil, denied)
			},
		},
		{
			name:    "settings",
			factory: settings.New,
			state: map[string]tftypes.Value{
				"id":       str("settings"),
				"settings": str(`{"keepDataPeriodDays":180}`),
			},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().GetSettings(gomock.Any()).Return(nil, denied)
			},
		},
		{
			name:    "status page groups",
			factory: statuspage.New,
			state:   map[string]tftypes.Value{"id": str("public")},
			expect: func(c *mocks.MockKumaClient) {
				// The configuration reads fine but the group tree, which comes
				// over HTTP, does not.
				c.EXPECT().GetStatusPage(gomock.Any(), "public").
					Return(&kuma.StatusPage{ID: 1, Slug: "public", Title: "T", Theme: "auto"}, nil)
				c.EXPECT().GetStatusPageGroups(gomock.Any(), "public").Return(nil, denied)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := mocks.NewMockKumaClient(gomock.NewController(t))
			tt.expect(client)

			r := configure(t, tt.factory, client)
			removed, errs := r.read(t, r.state(t, tt.state))

			if errs == "" {
				t.Error("a real failure should produce a diagnostic")
			}
			if removed {
				t.Error("a failed read must not drop the resource from state")
			}
		})
	}
}

// TestDeleteToleratesAlreadyGone covers the other end of the lifecycle:
// destroying something that is already gone is a success.
func TestDeleteToleratesAlreadyGone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		factory func() fwresource.Resource
		state   map[string]tftypes.Value
		expect  func(*mocks.MockKumaClient)
	}{
		{
			name:    "monitor",
			factory: monitor.NewPortResource,
			state:   map[string]tftypes.Value{"id": str("7")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().DeleteMonitor(gomock.Any(), 7, false).Return(kuma.ErrNotFound)
			},
		},
		{
			name:    "tag",
			factory: tag.New,
			state:   map[string]tftypes.Value{"id": str("3")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().DeleteTag(gomock.Any(), 3).Return(kuma.ErrNotFound)
			},
		},
		{
			name:    "notification",
			factory: notification.New,
			state:   map[string]tftypes.Value{"id": str("5")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().DeleteNotification(gomock.Any(), 5).Return(kuma.ErrNotFound)
			},
		},
		{
			name:    "proxy",
			factory: proxy.New,
			state:   map[string]tftypes.Value{"id": str("2")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().DeleteProxy(gomock.Any(), 2).Return(kuma.ErrNotFound)
			},
		},
		{
			name:    "docker host",
			factory: dockerhost.New,
			state:   map[string]tftypes.Value{"id": str("1")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().DeleteDockerHost(gomock.Any(), 1).Return(kuma.ErrNotFound)
			},
		},
		{
			name:    "remote browser",
			factory: remotebrowser.New,
			state:   map[string]tftypes.Value{"id": str("1")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().DeleteRemoteBrowser(gomock.Any(), 1).Return(kuma.ErrNotFound)
			},
		},
		{
			name:    "api key",
			factory: apikey.New,
			state:   map[string]tftypes.Value{"id": str("4")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().DeleteAPIKey(gomock.Any(), 4).Return(kuma.ErrNotFound)
			},
		},
		{
			name:    "maintenance",
			factory: maintenance.New,
			state:   map[string]tftypes.Value{"id": str("6")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().DeleteMaintenance(gomock.Any(), 6).Return(kuma.ErrNotFound)
			},
		},
		{
			name:    "status page",
			factory: statuspage.New,
			state:   map[string]tftypes.Value{"id": str("public")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().DeleteStatusPage(gomock.Any(), "public").Return(kuma.ErrNotFound)
			},
		},
		{
			name:    "status page incident",
			factory: statuspageincident.New,
			state:   map[string]tftypes.Value{"id": str("public/9")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().DeleteIncident(gomock.Any(), "public", 9).Return(kuma.ErrNotFound)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := mocks.NewMockKumaClient(gomock.NewController(t))
			tt.expect(client)

			r := configure(t, tt.factory, client)
			if errs := r.delete(t, r.state(t, tt.state)); errs != "" {
				t.Errorf("destroying something already gone should succeed: %s", errs)
			}
		})
	}
}

// TestDeleteSurfacesRealFailures is the other half of the pair above: every
// Delete tolerates not-found, and the risk is that the same branch swallows a
// genuine rejection too. Terraform would then drop the resource from state while
// it still exists on the server, and the next apply would collide with it.
//
// Each resource is checked, because the tolerate-and-report branch is written
// once per resource and one of them getting the condition backwards is invisible
// otherwise.
func TestDeleteSurfacesRealFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		factory func() fwresource.Resource
		state   map[string]tftypes.Value
		expect  func(*mocks.MockKumaClient)
	}{
		{
			name:    "monitor",
			factory: monitor.NewDNSResource,
			state:   map[string]tftypes.Value{"id": str("7")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().DeleteMonitor(gomock.Any(), 7, false).Return(denied)
			},
		},
		{
			name:    "tag",
			factory: tag.New,
			state:   map[string]tftypes.Value{"id": str("3")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().DeleteTag(gomock.Any(), 3).Return(denied)
			},
		},
		{
			name:    "notification",
			factory: notification.New,
			state:   map[string]tftypes.Value{"id": str("5")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().DeleteNotification(gomock.Any(), 5).Return(denied)
			},
		},
		{
			name:    "proxy",
			factory: proxy.New,
			state:   map[string]tftypes.Value{"id": str("2")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().DeleteProxy(gomock.Any(), 2).Return(denied)
			},
		},
		{
			name:    "docker host",
			factory: dockerhost.New,
			state:   map[string]tftypes.Value{"id": str("1")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().DeleteDockerHost(gomock.Any(), 1).Return(denied)
			},
		},
		{
			name:    "remote browser",
			factory: remotebrowser.New,
			state:   map[string]tftypes.Value{"id": str("1")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().DeleteRemoteBrowser(gomock.Any(), 1).Return(denied)
			},
		},
		{
			name:    "api key",
			factory: apikey.New,
			state:   map[string]tftypes.Value{"id": str("4")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().DeleteAPIKey(gomock.Any(), 4).Return(denied)
			},
		},
		{
			name:    "maintenance",
			factory: maintenance.New,
			state:   map[string]tftypes.Value{"id": str("6")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().DeleteMaintenance(gomock.Any(), 6).Return(denied)
			},
		},
		{
			name:    "status page",
			factory: statuspage.New,
			state:   map[string]tftypes.Value{"id": str("public")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().DeleteStatusPage(gomock.Any(), "public").Return(denied)
			},
		},
		{
			name:    "status page incident",
			factory: statuspageincident.New,
			state:   map[string]tftypes.Value{"id": str("public/9")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().DeleteIncident(gomock.Any(), "public", 9).Return(denied)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := mocks.NewMockKumaClient(gomock.NewController(t))
			tt.expect(client)

			r := configure(t, tt.factory, client)
			errs := r.delete(t, r.state(t, tt.state))
			if errs == "" {
				t.Fatal("a permission failure should produce a diagnostic, not a silent " +
					"removal from state")
			}
			if !strings.Contains(errs, "Permission denied") {
				t.Errorf("the server's message should survive: %s", errs)
			}
		})
	}
}

// TestInvalidStateIDs covers the guard for a state that cannot be parsed, which
// happens after a hand-edited state file or a botched import.
func TestInvalidStateIDs(t *testing.T) {
	t.Parallel()

	t.Run("a non-numeric monitor ID is rejected", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		r := configure(t, monitor.NewKeywordResource, client)

		_, errs := r.read(t, r.state(t, map[string]tftypes.Value{"id": str("not-a-number")}))
		if errs == "" {
			t.Error("expected a diagnostic")
		}
	})

	t.Run("an incident ID without a slug is rejected", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		r := configure(t, statuspageincident.New, client)

		_, errs := r.read(t, r.state(t, map[string]tftypes.Value{"id": str("9")}))
		if errs == "" {
			t.Error("expected a diagnostic for a missing slug")
		}
	})

	t.Run("an incident ID with a non-numeric part is rejected", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		r := configure(t, statuspageincident.New, client)

		_, errs := r.read(t, r.state(t, map[string]tftypes.Value{"id": str("public/abc")}))
		if errs == "" {
			t.Error("expected a diagnostic")
		}
	})
}

// TestImportStateValidation covers the import guards, which reject an ID in the
// wrong shape before anything touches the server.
func TestImportStateValidation(t *testing.T) {
	t.Parallel()

	t.Run("monitors need a numeric ID", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		r := configure(t, monitor.NewHTTPResource, client)

		if errs := r.importState(t, "not-a-number"); errs == "" {
			t.Error("expected a diagnostic")
		}
		if errs := r.importState(t, "12"); errs != "" {
			t.Errorf("a numeric ID should be accepted: %s", errs)
		}
	})

	t.Run("status pages need a slug, not a number", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		r := configure(t, statuspage.New, client)

		// Passing a number is the likely mistake, since every other resource in
		// the provider is imported that way.
		if errs := r.importState(t, "42"); errs == "" {
			t.Error("a numeric ID should be rejected with an explanation")
		}
		if errs := r.importState(t, "public"); errs != "" {
			t.Errorf("a slug should be accepted: %s", errs)
		}
	})

	t.Run("incidents need the composite form", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		r := configure(t, statuspageincident.New, client)

		if errs := r.importState(t, "9"); errs == "" {
			t.Error("expected a diagnostic")
		}
		if errs := r.importState(t, "public/9"); errs != "" {
			t.Errorf("the composite form should be accepted: %s", errs)
		}
	})
}

// TestCreateSurfacesFailures covers the create path when the server says no.
func TestCreateSurfacesFailures(t *testing.T) {
	t.Parallel()

	t.Run("monitor", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().CreateMonitor(gomock.Any(), gomock.Any()).
			Return(0, &kuma.APIError{Event: "add", Msg: "Retry interval cannot be less than 1 seconds"})

		r := configure(t, monitor.NewHTTPResource, client)
		plan := r.state(t, map[string]tftypes.Value{
			"name": str("x"),
			"url":  str("https://example.com"),
		})
		if errs := r.create(t, plan); errs == "" {
			t.Error("expected a diagnostic")
		}
	})

	t.Run("tag", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().CreateTag(gomock.Any(), gomock.Any()).Return(0, denied)

		r := configure(t, tag.New, client)
		plan := r.state(t, map[string]tftypes.Value{
			"name":  str("x"),
			"color": str("#000000"),
		})
		if errs := r.create(t, plan); errs == "" {
			t.Error("expected a diagnostic")
		}
	})

	t.Run("status page reports a half-created page clearly", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		// Creation succeeds, then the save that configures it fails. The page
		// exists but is empty, and the message has to say so rather than leave
		// the user wondering why the next plan rewrites everything.
		client.EXPECT().CreateStatusPage(gomock.Any(), "T", "public").Return("public", nil)
		client.EXPECT().SaveStatusPage(gomock.Any(), "public", gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, denied)

		r := configure(t, statuspage.New, client)
		plan := r.state(t, map[string]tftypes.Value{
			"slug":  str("public"),
			"title": str("T"),
		})
		errs := r.create(t, plan)
		if errs == "" {
			t.Fatal("expected a diagnostic")
		}
		if !strings.Contains(errs, "created but not configured") {
			t.Errorf("the message should explain the half-done state: %s", errs)
		}
	})

	t.Run("notification rejects settings that fight the schema", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		// No client call at all: the payload never gets built.
		r := configure(t, notification.New, client)
		plan := r.state(t, map[string]tftypes.Value{
			"name":     str("x"),
			"type":     str("webhook"),
			"settings": str(`{"name":"override"}`),
		})
		errs := r.create(t, plan)
		if errs == "" {
			t.Fatal("expected a diagnostic")
		}
		if !strings.Contains(errs, "must not contain") {
			t.Errorf("the message should name the offending key: %s", errs)
		}
	})

	t.Run("settings refuses disableAuth", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		r := configure(t, settings.New, client)
		plan := r.state(t, map[string]tftypes.Value{
			"settings": str(`{"disableAuth":true}`),
		})
		errs := r.create(t, plan)
		if errs == "" {
			t.Fatal("expected a diagnostic")
		}
		if !strings.Contains(errs, "disableAuth") {
			t.Errorf("the message should name the setting: %s", errs)
		}
	})
}

// TestSettingsDeleteWarns documents the singleton behaviour: destroying the
// resource cannot revert the values, so it warns instead of pretending.
func TestSettingsDeleteWarns(t *testing.T) {
	t.Parallel()

	client := mocks.NewMockKumaClient(gomock.NewController(t))
	r := configure(t, settings.New, client)

	state := r.state(t, map[string]tftypes.Value{
		"id":       str("settings"),
		"settings": str(`{"keepDataPeriodDays":180}`),
	})

	resp := &fwresource.DeleteResponse{State: state}
	r.res.Delete(context.Background(), fwresource.DeleteRequest{State: state}, resp)

	if resp.Diagnostics.HasError() {
		t.Errorf("destroying settings is not an error: %s", resp.Diagnostics)
	}
	if resp.Diagnostics.WarningsCount() == 0 {
		t.Error("the user should be told the values were left in place")
	}
}
