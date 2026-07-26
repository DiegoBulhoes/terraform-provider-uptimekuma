package resource_test

import (
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

// Update is the method with the most branches and the least acceptance coverage:
// a passing acceptance test only ever walks the success path. These drive the
// failures.

func TestUpdateSurfacesFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		factory func() fwresource.Resource
		state   map[string]tftypes.Value
		plan    map[string]tftypes.Value
		expect  func(*mocks.MockKumaClient)
	}{
		{
			name:    "monitor",
			factory: monitor.NewHTTPResource,
			state:   map[string]tftypes.Value{"id": str("7"), "name": str("old"), "url": str("https://a")},
			plan:    map[string]tftypes.Value{"id": str("7"), "name": str("new"), "url": str("https://b")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().UpdateMonitor(gomock.Any(), gomock.Any()).Return(denied)
			},
		},
		{
			name:    "tag",
			factory: tag.New,
			state:   map[string]tftypes.Value{"id": str("3"), "name": str("old"), "color": str("#000000")},
			plan:    map[string]tftypes.Value{"id": str("3"), "name": str("new"), "color": str("#ffffff")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().UpdateTag(gomock.Any(), gomock.Any()).Return(denied)
			},
		},
		{
			name:    "notification",
			factory: notification.New,
			state: map[string]tftypes.Value{
				"id": str("5"), "name": str("old"), "type": str("webhook"),
				"settings": str(`{"webhookURL":"https://a"}`),
			},
			plan: map[string]tftypes.Value{
				"id": str("5"), "name": str("new"), "type": str("webhook"),
				"settings": str(`{"webhookURL":"https://b"}`),
			},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().SaveNotification(gomock.Any(), gomock.Any(), gomock.Any()).Return(0, denied)
			},
		},
		{
			name:    "proxy",
			factory: proxy.New,
			state: map[string]tftypes.Value{
				"id": str("2"), "protocol": str("http"), "host": str("a"), "port": num(3128),
			},
			plan: map[string]tftypes.Value{
				"id": str("2"), "protocol": str("http"), "host": str("b"), "port": num(3128),
			},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().SaveProxy(gomock.Any(), gomock.Any(), gomock.Any()).Return(0, denied)
			},
		},
		{
			name:    "docker host",
			factory: dockerhost.New,
			state: map[string]tftypes.Value{
				"id": str("1"), "name": str("old"), "connection_type": str("socket"), "daemon": str("/a"),
			},
			plan: map[string]tftypes.Value{
				"id": str("1"), "name": str("new"), "connection_type": str("socket"), "daemon": str("/b"),
			},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().SaveDockerHost(gomock.Any(), gomock.Any(), gomock.Any()).Return(0, denied)
			},
		},
		{
			name:    "remote browser",
			factory: remotebrowser.New,
			state:   map[string]tftypes.Value{"id": str("1"), "name": str("old"), "url": str("ws://a")},
			plan:    map[string]tftypes.Value{"id": str("1"), "name": str("new"), "url": str("ws://b")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().SaveRemoteBrowser(gomock.Any(), gomock.Any(), gomock.Any()).Return(0, denied)
			},
		},
		{
			name:    "maintenance",
			factory: maintenance.New,
			state: map[string]tftypes.Value{
				"id": str("6"), "title": str("old"), "description": str("d"), "strategy": str("manual"),
			},
			plan: map[string]tftypes.Value{
				"id": str("6"), "title": str("new"), "description": str("d"), "strategy": str("manual"),
			},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().UpdateMaintenance(gomock.Any(), gomock.Any()).Return(denied)
			},
		},
		{
			name:    "status page",
			factory: statuspage.New,
			state:   map[string]tftypes.Value{"id": str("public"), "slug": str("public"), "title": str("old")},
			plan:    map[string]tftypes.Value{"id": str("public"), "slug": str("public"), "title": str("new")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().SaveStatusPage(gomock.Any(), "public", gomock.Any(), gomock.Any(), gomock.Any()).
					Return(nil, denied)
			},
		},
		{
			name:    "status page incident",
			factory: statuspageincident.New,
			state: map[string]tftypes.Value{
				"id": str("public/9"), "status_page_slug": str("public"),
				"title": str("old"), "content": str("c"), "style": str("info"),
			},
			plan: map[string]tftypes.Value{
				"id": str("public/9"), "status_page_slug": str("public"),
				"title": str("new"), "content": str("c"), "style": str("info"),
			},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().EditIncident(gomock.Any(), "public", 9, gomock.Any()).Return(nil, denied)
			},
		},
		{
			name:    "settings",
			factory: settings.New,
			state: map[string]tftypes.Value{
				"id": str("settings"), "settings": str(`{"keepDataPeriodDays":180}`),
			},
			plan: map[string]tftypes.Value{
				"id": str("settings"), "settings": str(`{"keepDataPeriodDays":200}`),
			},
			expect: func(c *mocks.MockKumaClient) {
				// Settings are merged onto what the server has, so the read comes
				// first and is where this one fails.
				c.EXPECT().GetSettings(gomock.Any()).Return(nil, denied)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := mocks.NewMockKumaClient(gomock.NewController(t))
			tt.expect(client)

			r := configure(t, tt.factory, client)
			errs := r.update(t, r.state(t, tt.plan), r.state(t, tt.state))
			if errs == "" {
				t.Error("a failed update should produce a diagnostic")
			}
		})
	}
}

// TestUpdateReadBackFailures covers the second half of an update: the write
// succeeded but reading the result back did not. Without this branch the resource
// would report success while state went stale.
func TestUpdateReadBackFailures(t *testing.T) {
	t.Parallel()

	t.Run("monitor", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().UpdateMonitor(gomock.Any(), gomock.Any()).Return(nil)
		// reconcileActive reads current state before deciding to pause or resume.
		client.EXPECT().GetMonitor(gomock.Any(), 7).Return(nil, denied).AnyTimes()

		r := configure(t, monitor.NewHTTPResource, client)
		state := r.state(t, map[string]tftypes.Value{"id": str("7"), "name": str("old"), "url": str("https://a")})
		plan := r.state(t, map[string]tftypes.Value{
			"id": str("7"), "name": str("new"), "url": str("https://b"), "active": boolean(true),
		})

		if errs := r.update(t, plan, state); errs == "" {
			t.Error("a failed read-back should produce a diagnostic")
		}
	})

	t.Run("maintenance associations", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().UpdateMaintenance(gomock.Any(), gomock.Any()).Return(nil)
		// Both association sets are replace-all, so both are always written.
		client.EXPECT().SetMaintenanceMonitors(gomock.Any(), 6, gomock.Any()).Return(denied)

		r := configure(t, maintenance.New, client)
		state := r.state(t, map[string]tftypes.Value{
			"id": str("6"), "title": str("old"), "description": str("d"), "strategy": str("manual"),
		})
		plan := r.state(t, map[string]tftypes.Value{
			"id": str("6"), "title": str("new"), "description": str("d"), "strategy": str("manual"),
		})

		if errs := r.update(t, plan, state); errs == "" {
			t.Error("a failed association write should produce a diagnostic")
		}
	})
}

// TestMonitorPauseReconciliation covers the pause and resume path, which exists
// because editMonitor never writes the active column.
func TestMonitorPauseReconciliation(t *testing.T) {
	t.Parallel()

	t.Run("pausing calls pauseMonitor", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().UpdateMonitor(gomock.Any(), gomock.Any()).Return(nil)
		// The server still reports it active, so a pause is needed.
		client.EXPECT().GetMonitor(gomock.Any(), 7).
			Return(&kuma.Monitor{ID: 7, Name: "m", Type: "http", Active: kuma.BoolPtr(true)}, nil).
			Times(2)
		client.EXPECT().PauseMonitor(gomock.Any(), 7).Return(nil)

		r := configure(t, monitor.NewHTTPResource, client)
		state := r.state(t, map[string]tftypes.Value{"id": str("7"), "name": str("m"), "url": str("https://a")})
		plan := r.state(t, map[string]tftypes.Value{
			"id": str("7"), "name": str("m"), "url": str("https://a"), "active": boolean(false),
		})

		if errs := r.update(t, plan, state); errs != "" {
			t.Errorf("unexpected diagnostics: %s", errs)
		}
	})

	t.Run("resuming calls resumeMonitor", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().UpdateMonitor(gomock.Any(), gomock.Any()).Return(nil)
		client.EXPECT().GetMonitor(gomock.Any(), 7).
			Return(&kuma.Monitor{ID: 7, Name: "m", Type: "http", Active: kuma.BoolPtr(false)}, nil).
			Times(2)
		client.EXPECT().ResumeMonitor(gomock.Any(), 7).Return(nil)

		r := configure(t, monitor.NewHTTPResource, client)
		state := r.state(t, map[string]tftypes.Value{"id": str("7"), "name": str("m"), "url": str("https://a")})
		plan := r.state(t, map[string]tftypes.Value{
			"id": str("7"), "name": str("m"), "url": str("https://a"), "active": boolean(true),
		})

		if errs := r.update(t, plan, state); errs != "" {
			t.Errorf("unexpected diagnostics: %s", errs)
		}
	})

	t.Run("no change means no call", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().UpdateMonitor(gomock.Any(), gomock.Any()).Return(nil)
		client.EXPECT().GetMonitor(gomock.Any(), 7).
			Return(&kuma.Monitor{ID: 7, Name: "m", Type: "http", Active: kuma.BoolPtr(true)}, nil).
			Times(2)
		// Neither pause nor resume is expected: gomock fails the test if one is
		// called.

		r := configure(t, monitor.NewHTTPResource, client)
		state := r.state(t, map[string]tftypes.Value{"id": str("7"), "name": str("m"), "url": str("https://a")})
		plan := r.state(t, map[string]tftypes.Value{
			"id": str("7"), "name": str("m"), "url": str("https://a"), "active": boolean(true),
		})

		if errs := r.update(t, plan, state); errs != "" {
			t.Errorf("unexpected diagnostics: %s", errs)
		}
	})

	t.Run("a failed pause surfaces", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().UpdateMonitor(gomock.Any(), gomock.Any()).Return(nil)
		client.EXPECT().GetMonitor(gomock.Any(), 7).
			Return(&kuma.Monitor{ID: 7, Name: "m", Type: "http", Active: kuma.BoolPtr(true)}, nil)
		client.EXPECT().PauseMonitor(gomock.Any(), 7).Return(denied)

		r := configure(t, monitor.NewHTTPResource, client)
		state := r.state(t, map[string]tftypes.Value{"id": str("7"), "name": str("m"), "url": str("https://a")})
		plan := r.state(t, map[string]tftypes.Value{
			"id": str("7"), "name": str("m"), "url": str("https://a"), "active": boolean(false),
		})

		if errs := r.update(t, plan, state); errs == "" {
			t.Error("expected a diagnostic")
		}
	})
}

// TestAPIKeyUpdateOnlyTogglesActive documents that the API has no edit event for
// keys: only the enable and disable toggles, so nothing else can change in place.
func TestAPIKeyUpdateOnlyTogglesActive(t *testing.T) {
	t.Parallel()

	client := mocks.NewMockKumaClient(gomock.NewController(t))
	client.EXPECT().SetAPIKeyActive(gomock.Any(), 4, false).Return(nil)
	client.EXPECT().GetAPIKey(gomock.Any(), 4).
		Return(&kuma.APIKey{ID: 4, Name: "k", Active: kuma.Bool(false), Status: "inactive"}, nil)

	r := configure(t, apikey.New, client)
	state := r.state(t, map[string]tftypes.Value{
		"id": str("4"), "name": str("k"), "active": boolean(true), "key": str("uk4_secret"),
	})
	plan := r.state(t, map[string]tftypes.Value{
		"id": str("4"), "name": str("k"), "active": boolean(false),
	})

	if errs := r.update(t, plan, state); errs != "" {
		t.Errorf("unexpected diagnostics: %s", errs)
	}
}
