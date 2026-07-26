package resource_test

import (
	"testing"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/maintenance"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/monitor"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/statuspage"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/test/mocks"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"go.uber.org/mock/gomock"
)

// The read-back after a write.
//
// Create and Update both write, then read the object back to fill in whatever the
// server computed — a derived cron expression, a monitor's own URL, the push
// token. If that read fails the operation has to fail too, even though the write
// itself succeeded.
//
// It is tempting to ignore the error and keep the plan values instead, since the
// object does exist by then. That is the worse outcome: the state would record
// values the server never confirmed, and every later plan would diff against
// them. Failing leaves the object present with a tainted resource, which the next
// apply reconciles.

func TestCreateFailsWhenTheReadBackFails(t *testing.T) {
	t.Parallel()

	t.Run("monitor", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().CreateMonitor(gomock.Any(), gomock.Any()).Return(7, nil)
		client.EXPECT().GetMonitor(gomock.Any(), 7).Return(nil, denied)

		r := configure(t, monitor.NewHTTPResource, client)
		errs := r.create(t, r.state(t, map[string]tftypes.Value{
			"name": str("api"), "url": str("https://example.com"),
		}))
		if errs == "" {
			t.Error("a failed read-back must fail the create: keeping the plan values " +
				"would record what the server never confirmed")
		}
	})

	t.Run("maintenance", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().CreateMaintenance(gomock.Any(), gomock.Any()).Return(6, nil)
		client.EXPECT().SetMaintenanceMonitors(gomock.Any(), 6, gomock.Any()).Return(nil).AnyTimes()
		client.EXPECT().SetMaintenanceStatusPages(gomock.Any(), 6, gomock.Any()).Return(nil).AnyTimes()
		client.EXPECT().GetMaintenance(gomock.Any(), 6).Return(nil, denied)

		r := configure(t, maintenance.New, client)
		errs := r.create(t, r.state(t, map[string]tftypes.Value{
			"title": str("window"), "strategy": str("manual"),
		}))
		if errs == "" {
			t.Error("a failed read-back must fail the create")
		}
	})

	t.Run("status page", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().CreateStatusPage(gomock.Any(), gomock.Any(), gomock.Any()).Return("public", nil)
		client.EXPECT().SaveStatusPage(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, nil).AnyTimes()
		client.EXPECT().GetStatusPage(gomock.Any(), "public").Return(nil, denied)

		r := configure(t, statuspage.New, client)
		errs := r.create(t, r.state(t, map[string]tftypes.Value{
			"slug": str("public"), "title": str("Status"),
		}))
		if errs == "" {
			t.Error("a failed read-back must fail the create")
		}
	})
}

func TestUpdateFailsWhenTheReadBackFails(t *testing.T) {
	t.Parallel()

	t.Run("monitor", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().UpdateMonitor(gomock.Any(), gomock.Any()).Return(nil)
		client.EXPECT().GetMonitor(gomock.Any(), 7).Return(nil, denied).AnyTimes()

		r := configure(t, monitor.NewHTTPResource, client)
		state := r.state(t, map[string]tftypes.Value{
			"id": str("7"), "name": str("api"), "url": str("https://example.com"),
		})
		plan := r.state(t, map[string]tftypes.Value{
			"id": str("7"), "name": str("api renamed"), "url": str("https://example.com"),
		})

		if errs := r.update(t, plan, state); errs == "" {
			t.Error("a failed read-back must fail the update")
		}
	})

	t.Run("maintenance", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().UpdateMaintenance(gomock.Any(), gomock.Any()).Return(nil)
		client.EXPECT().SetMaintenanceMonitors(gomock.Any(), 6, gomock.Any()).Return(nil).AnyTimes()
		client.EXPECT().SetMaintenanceStatusPages(gomock.Any(), 6, gomock.Any()).Return(nil).AnyTimes()
		client.EXPECT().GetMaintenance(gomock.Any(), 6).Return(nil, denied).AnyTimes()

		r := configure(t, maintenance.New, client)
		state := r.state(t, map[string]tftypes.Value{
			"id": str("6"), "title": str("window"), "strategy": str("manual"),
		})
		plan := r.state(t, map[string]tftypes.Value{
			"id": str("6"), "title": str("renamed"), "strategy": str("manual"),
		})

		if errs := r.update(t, plan, state); errs == "" {
			t.Error("a failed read-back must fail the update")
		}
	})

	t.Run("status page", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().SaveStatusPage(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).
			Return(nil, nil)
		client.EXPECT().GetStatusPage(gomock.Any(), "public").Return(nil, denied).AnyTimes()

		r := configure(t, statuspage.New, client)
		state := r.state(t, map[string]tftypes.Value{
			"id": str("public"), "slug": str("public"), "title": str("Status"),
		})
		plan := r.state(t, map[string]tftypes.Value{
			"id": str("public"), "slug": str("public"), "title": str("Renamed"),
		})

		if errs := r.update(t, plan, state); errs == "" {
			t.Error("a failed read-back must fail the update")
		}
	})
}

// TestAnObjectDeletedBetweenTheWriteAndTheReadBackIsReported covers the narrow
// race where someone removes the object in the window between the two calls. It
// has to be an error rather than a silent removal from state: the apply did
// create something, and reporting success with no resource would leave the user
// with no way to see what happened.
func TestAnObjectDeletedBetweenTheWriteAndTheReadBackIsReported(t *testing.T) {
	t.Parallel()

	client := mocks.NewMockKumaClient(gomock.NewController(t))
	client.EXPECT().CreateMonitor(gomock.Any(), gomock.Any()).Return(7, nil)
	client.EXPECT().GetMonitor(gomock.Any(), 7).Return(nil, gone)

	r := configure(t, monitor.NewPingResource, client)
	errs := r.create(t, r.state(t, map[string]tftypes.Value{
		"name": str("ping"), "hostname": str("127.0.0.1"),
	}))
	if errs == "" {
		t.Error("an object that vanished during the create must be reported, not " +
			"silently dropped")
	}
}

// TestTagReconciliationHandlesEveryTransition covers the set arithmetic that keeps
// a monitor's tags in sync. Uptime Kuma has no "replace all tags" event, so the
// resource computes the difference and issues one add or delete per change.
//
// The interesting part is what must NOT happen: a tag present in both plan and
// state has to be left alone. Removing and re-adding it would work, but the tag
// link carries a value, and the round trip would drop it.
func TestTagReconciliationHandlesEveryTransition(t *testing.T) {
	t.Parallel()

	tagObject := func(id int64, value string) tftypes.Value {
		return tftypes.NewValue(
			tftypes.Object{AttributeTypes: map[string]tftypes.Type{
				"tag_id": tftypes.Number,
				"value":  tftypes.String,
			}},
			map[string]tftypes.Value{
				"tag_id": tftypes.NewValue(tftypes.Number, id),
				"value":  tftypes.NewValue(tftypes.String, value),
			},
		)
	}
	tagSet := func(values ...tftypes.Value) tftypes.Value {
		return tftypes.NewValue(tftypes.Set{ElementType: tftypes.Object{
			AttributeTypes: map[string]tftypes.Type{
				"tag_id": tftypes.Number,
				"value":  tftypes.String,
			},
		}}, values)
	}

	t.Run("an unchanged tag is left alone", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().UpdateMonitor(gomock.Any(), gomock.Any()).Return(nil)
		// No AddMonitorTag and no DeleteMonitorTag: GoMock fails the test if either
		// is called, which is the assertion.
		client.EXPECT().GetMonitor(gomock.Any(), 7).Return(&kuma.Monitor{
			ID: 7, Name: "api", Type: "http", URL: ptrTo("https://example.com"),
			Interval: 60, Active: kuma.BoolPtr(true),
			Tags: []kuma.MonitorTag{{TagID: 1, Name: "env", Value: ptrTo("prod")}},
		}, nil).AnyTimes()

		r := configure(t, monitor.NewHTTPResource, client)
		state := r.state(t, map[string]tftypes.Value{
			"id": str("7"), "name": str("api"), "url": str("https://example.com"),
			"tags": tagSet(tagObject(1, "prod")),
		})
		plan := r.state(t, map[string]tftypes.Value{
			"id": str("7"), "name": str("api"), "url": str("https://example.com"),
			"tags": tagSet(tagObject(1, "prod")),
		})

		if errs := r.update(t, plan, state); errs != "" {
			t.Fatalf("updating: %s", errs)
		}
	})

	t.Run("a tag is added and another removed", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().UpdateMonitor(gomock.Any(), gomock.Any()).Return(nil)
		client.EXPECT().DeleteMonitorTag(gomock.Any(), 1, 7, "prod").Return(nil)
		client.EXPECT().AddMonitorTag(gomock.Any(), 2, 7, "team-a").Return(nil)
		client.EXPECT().GetMonitor(gomock.Any(), 7).Return(&kuma.Monitor{
			ID: 7, Name: "api", Type: "http", URL: ptrTo("https://example.com"),
			Interval: 60, Active: kuma.BoolPtr(true),
			Tags: []kuma.MonitorTag{{TagID: 2, Name: "team", Value: ptrTo("team-a")}},
		}, nil).AnyTimes()

		r := configure(t, monitor.NewHTTPResource, client)
		state := r.state(t, map[string]tftypes.Value{
			"id": str("7"), "name": str("api"), "url": str("https://example.com"),
			"tags": tagSet(tagObject(1, "prod")),
		})
		plan := r.state(t, map[string]tftypes.Value{
			"id": str("7"), "name": str("api"), "url": str("https://example.com"),
			"tags": tagSet(tagObject(2, "team-a")),
		})

		if errs := r.update(t, plan, state); errs != "" {
			t.Fatalf("updating: %s", errs)
		}
	})

	t.Run("changing only the value re-links the tag", func(t *testing.T) {
		t.Parallel()

		// The link is identified by tag ID and value together, so a new value is a
		// different link: the old one goes and a new one arrives.
		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().UpdateMonitor(gomock.Any(), gomock.Any()).Return(nil)
		client.EXPECT().DeleteMonitorTag(gomock.Any(), 1, 7, "staging").Return(nil)
		client.EXPECT().AddMonitorTag(gomock.Any(), 1, 7, "prod").Return(nil)
		client.EXPECT().GetMonitor(gomock.Any(), 7).Return(&kuma.Monitor{
			ID: 7, Name: "api", Type: "http", URL: ptrTo("https://example.com"),
			Interval: 60, Active: kuma.BoolPtr(true),
			Tags: []kuma.MonitorTag{{TagID: 1, Name: "env", Value: ptrTo("prod")}},
		}, nil).AnyTimes()

		r := configure(t, monitor.NewHTTPResource, client)
		state := r.state(t, map[string]tftypes.Value{
			"id": str("7"), "name": str("api"), "url": str("https://example.com"),
			"tags": tagSet(tagObject(1, "staging")),
		})
		plan := r.state(t, map[string]tftypes.Value{
			"id": str("7"), "name": str("api"), "url": str("https://example.com"),
			"tags": tagSet(tagObject(1, "prod")),
		})

		if errs := r.update(t, plan, state); errs != "" {
			t.Fatalf("updating: %s", errs)
		}
	})

	t.Run("a failing tag link surfaces", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().UpdateMonitor(gomock.Any(), gomock.Any()).Return(nil)
		client.EXPECT().AddMonitorTag(gomock.Any(), 2, 7, "team-a").Return(denied)
		client.EXPECT().GetMonitor(gomock.Any(), 7).Return(&kuma.Monitor{
			ID: 7, Name: "api", Type: "http", URL: ptrTo("https://example.com"),
			Interval: 60, Active: kuma.BoolPtr(true),
		}, nil).AnyTimes()

		r := configure(t, monitor.NewHTTPResource, client)
		state := r.state(t, map[string]tftypes.Value{
			"id": str("7"), "name": str("api"), "url": str("https://example.com"),
		})
		plan := r.state(t, map[string]tftypes.Value{
			"id": str("7"), "name": str("api"), "url": str("https://example.com"),
			"tags": tagSet(tagObject(2, "team-a")),
		})

		if errs := r.update(t, plan, state); errs == "" {
			t.Error("a failing tag link must surface: the monitor would otherwise be " +
				"reported as tagged when it is not")
		}
	})
}

func ptrTo[T any](v T) *T { return &v }
