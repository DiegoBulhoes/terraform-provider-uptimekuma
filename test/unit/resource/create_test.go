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
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"go.uber.org/mock/gomock"
)

// Create on the happy path, with the associations that only some configurations
// have. The acceptance tests create plain objects, so the branches that attach
// tags, notifications, monitors or status pages during creation are only walked
// here.

func TestCreateWithAssociations(t *testing.T) {
	t.Parallel()

	t.Run("monitor attaches its tags after the monitor exists", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().CreateMonitor(gomock.Any(), gomock.Any()).Return(7, nil)
		// Tags are separate associations, so they can only go on once the monitor
		// has an ID.
		client.EXPECT().AddMonitorTag(gomock.Any(), 10, 7, "production").Return(nil)
		client.EXPECT().GetMonitor(gomock.Any(), 7).
			Return(&kuma.Monitor{ID: 7, Name: "m", Type: "http", URL: ptr("https://a"), Active: kuma.BoolPtr(true)}, nil).
			AnyTimes()

		r := configure(t, monitor.NewHTTPResource, client)
		plan := r.state(t, map[string]tftypes.Value{
			"name": str("m"),
			"url":  str("https://a"),
			"tags": tagSetValue(t, 10, "production"),
		})

		if errs := r.create(t, plan); errs != "" {
			t.Errorf("unexpected diagnostics: %s", errs)
		}
	})

	t.Run("a failing tag attach surfaces", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().CreateMonitor(gomock.Any(), gomock.Any()).Return(7, nil)
		client.EXPECT().AddMonitorTag(gomock.Any(), 10, 7, "production").Return(denied)

		r := configure(t, monitor.NewHTTPResource, client)
		plan := r.state(t, map[string]tftypes.Value{
			"name": str("m"),
			"url":  str("https://a"),
			"tags": tagSetValue(t, 10, "production"),
		})

		if errs := r.create(t, plan); errs == "" {
			t.Error("expected a diagnostic")
		}
	})

	t.Run("maintenance attaches monitors and status pages", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().CreateMaintenance(gomock.Any(), gomock.Any()).Return(6, nil)
		client.EXPECT().SetMaintenanceMonitors(gomock.Any(), 6, []int{11}).Return(nil)
		client.EXPECT().SetMaintenanceStatusPages(gomock.Any(), 6, []int{2}).Return(nil)
		client.EXPECT().GetMaintenance(gomock.Any(), 6).Return(&kuma.Maintenance{
			ID: 6, Title: "w", Description: "d", Strategy: "manual",
			Active: kuma.BoolPtr(true), DateRange: []*string{nil, nil},
		}, nil)
		client.EXPECT().GetMaintenanceMonitors(gomock.Any(), 6).Return([]int{11}, nil)
		client.EXPECT().GetMaintenanceStatusPages(gomock.Any(), 6).Return([]int{2}, nil)

		r := configure(t, maintenance.New, client)
		plan := r.state(t, map[string]tftypes.Value{
			"title":           str("w"),
			"description":     str("d"),
			"strategy":        str("manual"),
			"monitor_ids":     int64SetValue(t, 11),
			"status_page_ids": int64SetValue(t, 2),
		})

		if errs := r.create(t, plan); errs != "" {
			t.Errorf("unexpected diagnostics: %s", errs)
		}
	})

	t.Run("a failing monitor attach on a new window surfaces", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().CreateMaintenance(gomock.Any(), gomock.Any()).Return(6, nil)
		client.EXPECT().SetMaintenanceMonitors(gomock.Any(), 6, []int{11}).Return(denied)

		r := configure(t, maintenance.New, client)
		plan := r.state(t, map[string]tftypes.Value{
			"title":       str("w"),
			"description": str("d"),
			"strategy":    str("manual"),
			"monitor_ids": int64SetValue(t, 11),
		})

		if errs := r.create(t, plan); errs == "" {
			t.Error("expected a diagnostic")
		}
	})

	t.Run("a failing status page attach on a new window surfaces", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().CreateMaintenance(gomock.Any(), gomock.Any()).Return(6, nil)
		client.EXPECT().SetMaintenanceStatusPages(gomock.Any(), 6, []int{2}).Return(denied)

		r := configure(t, maintenance.New, client)
		plan := r.state(t, map[string]tftypes.Value{
			"title":           str("w"),
			"description":     str("d"),
			"strategy":        str("manual"),
			"status_page_ids": int64SetValue(t, 2),
		})

		if errs := r.create(t, plan); errs == "" {
			t.Error("expected a diagnostic")
		}
	})
}

func TestCreateSucceeds(t *testing.T) {
	t.Parallel()

	t.Run("tag", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().CreateTag(gomock.Any(), kuma.Tag{Name: "env", Color: "#000000"}).Return(3, nil)

		r := configure(t, tag.New, client)
		plan := r.state(t, map[string]tftypes.Value{"name": str("env"), "color": str("#000000")})
		if errs := r.create(t, plan); errs != "" {
			t.Errorf("unexpected diagnostics: %s", errs)
		}
	})

	t.Run("notification", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().SaveNotification(gomock.Any(), nil, gomock.Any()).Return(5, nil)
		client.EXPECT().GetNotification(gomock.Any(), 5).Return(&kuma.Notification{
			ID: 5, Name: "hook", Config: `{"name":"hook","type":"webhook","webhookURL":"https://a"}`,
		}, nil)

		r := configure(t, notification.New, client)
		plan := r.state(t, map[string]tftypes.Value{
			"name": str("hook"), "type": str("webhook"),
			"settings": str(`{"webhookURL":"https://a"}`),
		})
		if errs := r.create(t, plan); errs != "" {
			t.Errorf("unexpected diagnostics: %s", errs)
		}
	})

	t.Run("notification read-back failure surfaces", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().SaveNotification(gomock.Any(), nil, gomock.Any()).Return(5, nil)
		client.EXPECT().GetNotification(gomock.Any(), 5).Return(nil, denied)

		r := configure(t, notification.New, client)
		plan := r.state(t, map[string]tftypes.Value{
			"name": str("hook"), "type": str("webhook"),
			"settings": str(`{"webhookURL":"https://a"}`),
		})
		if errs := r.create(t, plan); errs == "" {
			t.Error("expected a diagnostic")
		}
	})

	t.Run("proxy", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().SaveProxy(gomock.Any(), nil, gomock.Any()).Return(2, nil)
		client.EXPECT().GetProxy(gomock.Any(), 2).Return(&kuma.Proxy{
			ID: 2, Protocol: "http", Host: "p", Port: 3128, Active: kuma.Bool(true),
		}, nil)

		r := configure(t, proxy.New, client)
		plan := r.state(t, map[string]tftypes.Value{
			"protocol": str("http"), "host": str("p"), "port": num(3128),
		})
		if errs := r.create(t, plan); errs != "" {
			t.Errorf("unexpected diagnostics: %s", errs)
		}
	})

	t.Run("proxy read-back failure surfaces", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().SaveProxy(gomock.Any(), nil, gomock.Any()).Return(2, nil)
		client.EXPECT().GetProxy(gomock.Any(), 2).Return(nil, denied)

		r := configure(t, proxy.New, client)
		plan := r.state(t, map[string]tftypes.Value{
			"protocol": str("http"), "host": str("p"), "port": num(3128),
		})
		if errs := r.create(t, plan); errs == "" {
			t.Error("expected a diagnostic")
		}
	})

	t.Run("docker host", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().SaveDockerHost(gomock.Any(), nil, gomock.Any()).Return(1, nil)
		client.EXPECT().GetDockerHost(gomock.Any(), 1).Return(&kuma.DockerHost{
			ID: 1, Name: "d", DockerType: "socket", DockerDaemon: "/s",
		}, nil)

		r := configure(t, dockerhost.New, client)
		plan := r.state(t, map[string]tftypes.Value{
			"name": str("d"), "connection_type": str("socket"), "daemon": str("/s"),
		})
		if errs := r.create(t, plan); errs != "" {
			t.Errorf("unexpected diagnostics: %s", errs)
		}
	})

	t.Run("docker host read-back failure surfaces", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().SaveDockerHost(gomock.Any(), nil, gomock.Any()).Return(1, nil)
		client.EXPECT().GetDockerHost(gomock.Any(), 1).Return(nil, denied)

		r := configure(t, dockerhost.New, client)
		plan := r.state(t, map[string]tftypes.Value{
			"name": str("d"), "connection_type": str("socket"), "daemon": str("/s"),
		})
		if errs := r.create(t, plan); errs == "" {
			t.Error("expected a diagnostic")
		}
	})

	t.Run("remote browser", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().SaveRemoteBrowser(gomock.Any(), nil, gomock.Any()).Return(1, nil)

		r := configure(t, remotebrowser.New, client)
		plan := r.state(t, map[string]tftypes.Value{"name": str("b"), "url": str("ws://a")})
		if errs := r.create(t, plan); errs != "" {
			t.Errorf("unexpected diagnostics: %s", errs)
		}
	})

	t.Run("api key returns the clear-text secret", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().CreateAPIKey(gomock.Any(), gomock.Any()).Return(4, "uk4_secret", nil)
		client.EXPECT().GetAPIKey(gomock.Any(), 4).Return(&kuma.APIKey{
			ID: 4, Name: "k", Active: kuma.Bool(true), Status: "active",
		}, nil)

		r := configure(t, apikey.New, client)
		plan := r.state(t, map[string]tftypes.Value{"name": str("k")})
		if errs := r.create(t, plan); errs != "" {
			t.Errorf("unexpected diagnostics: %s", errs)
		}
	})

	t.Run("api key read-back failure surfaces", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().CreateAPIKey(gomock.Any(), gomock.Any()).Return(4, "uk4_secret", nil)
		client.EXPECT().GetAPIKey(gomock.Any(), 4).Return(nil, denied)

		r := configure(t, apikey.New, client)
		plan := r.state(t, map[string]tftypes.Value{"name": str("k")})
		if errs := r.create(t, plan); errs == "" {
			t.Error("expected a diagnostic")
		}
	})

	t.Run("status page with a group tree", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().CreateStatusPage(gomock.Any(), "Status", "public").Return("public", nil)
		client.EXPECT().SaveStatusPage(gomock.Any(), "public", gomock.Any(), gomock.Any(), gomock.Any()).
			Return([]kuma.StatusPageGroup{{ID: 1, Name: "Core"}}, nil)
		client.EXPECT().GetStatusPage(gomock.Any(), "public").Return(&kuma.StatusPage{
			ID: 1, Slug: "public", Title: "Status", Theme: "auto",
		}, nil)
		client.EXPECT().GetStatusPageGroups(gomock.Any(), "public").Return([]kuma.StatusPageGroup{
			{ID: 1, Name: "Core", MonitorList: []kuma.StatusPageMonitor{{ID: 10}}},
		}, nil)

		r := configure(t, statuspage.New, client)
		plan := r.state(t, map[string]tftypes.Value{"slug": str("public"), "title": str("Status")})
		if errs := r.create(t, plan); errs != "" {
			t.Errorf("unexpected diagnostics: %s", errs)
		}
	})

	t.Run("incident posted unpinned is resolved straight away", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		// Posting always pins, so asking for an unpinned incident needs a resolve
		// right after.
		client.EXPECT().PostIncident(gomock.Any(), "public", gomock.Any()).Return(
			&kuma.StatusPageIncident{ID: 9, Title: "t", Content: "c", Style: "info", Pin: kuma.BoolPtr(true)}, nil)
		client.EXPECT().ResolveIncident(gomock.Any(), "public", 9).Return(nil)

		r := configure(t, statuspageincident.New, client)
		plan := r.state(t, map[string]tftypes.Value{
			"status_page_slug": str("public"), "title": str("t"), "content": str("c"),
			"style": str("info"), "pinned": boolean(false),
		})
		if errs := r.create(t, plan); errs != "" {
			t.Errorf("unexpected diagnostics: %s", errs)
		}
	})

	t.Run("a failing resolve after posting surfaces", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().PostIncident(gomock.Any(), "public", gomock.Any()).Return(
			&kuma.StatusPageIncident{ID: 9, Pin: kuma.BoolPtr(true)}, nil)
		client.EXPECT().ResolveIncident(gomock.Any(), "public", 9).Return(denied)

		r := configure(t, statuspageincident.New, client)
		plan := r.state(t, map[string]tftypes.Value{
			"status_page_slug": str("public"), "title": str("t"), "content": str("c"),
			"pinned": boolean(false),
		})
		if errs := r.create(t, plan); errs == "" {
			t.Error("expected a diagnostic")
		}
	})

	t.Run("incident defaults to the warning style", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().PostIncident(gomock.Any(), "public", gomock.Any()).
			DoAndReturn(func(_ any, _ string, incident kuma.StatusPageIncident) (*kuma.StatusPageIncident, error) {
				if incident.Style != "warning" {
					t.Errorf("style = %q, want the warning default", incident.Style)
				}
				incident.ID = 9
				incident.Pin = kuma.BoolPtr(true)
				return &incident, nil
			})

		r := configure(t, statuspageincident.New, client)
		plan := r.state(t, map[string]tftypes.Value{
			"status_page_slug": str("public"), "title": str("t"), "content": str("c"),
		})
		if errs := r.create(t, plan); errs != "" {
			t.Errorf("unexpected diagnostics: %s", errs)
		}
	})

	t.Run("settings merges onto what the server holds", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		// setSettings replaces the whole document, so the current values are read
		// first and the managed keys merged on top. Writing only the managed keys
		// would wipe everything else.
		client.EXPECT().GetSettings(gomock.Any()).Return(map[string]any{
			"serverTimezone":     "UTC",
			"keepDataPeriodDays": float64(180),
		}, nil).Times(2)
		client.EXPECT().SetSettings(gomock.Any(), gomock.Any(), "").
			DoAndReturn(func(_ any, merged map[string]any, _ string) error {
				if _, kept := merged["serverTimezone"]; !kept {
					t.Error("an unmanaged setting was dropped")
				}
				if merged["keepDataPeriodDays"] != float64(200) {
					t.Errorf("the managed value did not win: %v", merged["keepDataPeriodDays"])
				}
				return nil
			})

		r := configure(t, settings.New, client)
		plan := r.state(t, map[string]tftypes.Value{"settings": str(`{"keepDataPeriodDays":200}`)})
		if errs := r.create(t, plan); errs != "" {
			t.Errorf("unexpected diagnostics: %s", errs)
		}
	})

	t.Run("a failing settings write surfaces", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().GetSettings(gomock.Any()).Return(map[string]any{}, nil)
		client.EXPECT().SetSettings(gomock.Any(), gomock.Any(), "").Return(denied)

		r := configure(t, settings.New, client)
		plan := r.state(t, map[string]tftypes.Value{"settings": str(`{"keepDataPeriodDays":200}`)})
		if errs := r.create(t, plan); errs == "" {
			t.Error("expected a diagnostic")
		}
	})
}

// tagSetValue builds the `tags` set of a monitor: a set of objects, not a plain
// list of IDs, because each association carries its own value.
func tagSetValue(t *testing.T, tagID int64, value string) tftypes.Value {
	t.Helper()

	object := tftypes.Object{AttributeTypes: map[string]tftypes.Type{
		"tag_id": tftypes.Number,
		"value":  tftypes.String,
	}}
	return tftypes.NewValue(tftypes.Set{ElementType: object}, []tftypes.Value{
		tftypes.NewValue(object, map[string]tftypes.Value{
			"tag_id": num(tagID),
			"value":  str(value),
		}),
	})
}

// int64SetValue builds a set of numbers, as used by every ID association.
func int64SetValue(t *testing.T, ids ...int64) tftypes.Value {
	t.Helper()

	elements := make([]tftypes.Value, 0, len(ids))
	for _, id := range ids {
		elements = append(elements, num(id))
	}
	return tftypes.NewValue(tftypes.Set{ElementType: tftypes.Number}, elements)
}
