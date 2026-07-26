package kuma_test

import (
	"encoding/json"
	"testing"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
)

// Happy-path tests: the shapes Uptime Kuma actually sends on a good day, and
// what the client is expected to make of them.

// TestHappyMonitorRoundTrip decodes a monitor payload the way getMonitor
// delivers it, in three variations of the same success case.
func TestHappyMonitorRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		json  string
		check func(*testing.T, kuma.Monitor)
	}{
		{
			name: "http monitor with the fields the web UI sets",
			json: `{
				"id": 12, "name": "API", "type": "http", "url": "https://api.example.com",
				"interval": 60, "retryInterval": 60, "maxretries": 3, "resendInterval": 0,
				"active": true, "accepted_statuscodes": ["200-299"], "method": "GET"
			}`,
			check: func(t *testing.T, m kuma.Monitor) {
				if m.ID != 12 || m.Name != "API" || m.Type != "http" {
					t.Errorf("identity did not survive: %+v", m)
				}
				if m.URL == nil || *m.URL != "https://api.example.com" {
					t.Errorf("url = %v", m.URL)
				}
				if !m.Active.Value() {
					t.Error("monitor should be active")
				}
				if len(m.AcceptedStatusCodes) != 1 {
					t.Errorf("accepted codes = %v", m.AcceptedStatusCodes)
				}
			},
		},
		{
			name: "ping monitor, where url is absent and hostname carries the target",
			json: `{
				"id": 3, "name": "Gateway", "type": "ping", "hostname": "10.0.0.1",
				"interval": 60, "retryInterval": 60, "packetSize": 56, "ping_numeric": true
			}`,
			check: func(t *testing.T, m kuma.Monitor) {
				if m.Hostname == nil || *m.Hostname != "10.0.0.1" {
					t.Errorf("hostname = %v", m.Hostname)
				}
				// An absent url must stay nil, not become an empty string.
				if m.URL != nil {
					t.Errorf("url should be nil, got %q", *m.URL)
				}
				if m.PacketSize == nil || *m.PacketSize != 56 {
					t.Errorf("packet size = %v", m.PacketSize)
				}
				if !m.PingNumeric.Value() {
					t.Error("ping_numeric should be true")
				}
			},
		},
		{
			name: "group monitor reporting its children and a parent link",
			json: `{
				"id": 7, "name": "Production", "type": "group", "interval": 60,
				"retryInterval": 60, "parent": 2, "childrenIDs": [8, 9, 10],
				"notificationIDList": {"1": true, "4": true}
			}`,
			check: func(t *testing.T, m kuma.Monitor) {
				if m.Parent == nil || *m.Parent != 2 {
					t.Errorf("parent = %v", m.Parent)
				}
				if len(m.ChildrenIDs) != 3 {
					t.Errorf("children = %v", m.ChildrenIDs)
				}
				// The notification set is an object keyed by stringified ID.
				if len(m.NotificationIDList) != 2 || !m.NotificationIDList["4"] {
					t.Errorf("notifications = %v", m.NotificationIDList)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var monitor kuma.Monitor
			if err := json.Unmarshal([]byte(tt.json), &monitor); err != nil {
				t.Fatalf("decoding: %v", err)
			}
			tt.check(t, monitor)
		})
	}
}

// TestHappyNormalizeMonitor covers the three defaults the client fills in
// because the server has none and rejects the payload without them.
func TestHappyNormalizeMonitor(t *testing.T) {
	t.Parallel()

	t.Run("status codes default to the 2xx range", func(t *testing.T) {
		t.Parallel()

		monitor := kuma.Monitor{Name: "web", Type: "http", Interval: 60}
		kuma.NormalizeMonitor(&monitor)

		if len(monitor.AcceptedStatusCodes) != 1 || monitor.AcceptedStatusCodes[0] != "200-299" {
			t.Errorf("accepted codes = %v", monitor.AcceptedStatusCodes)
		}
		// The handler JSON-stringifies both unconditionally, so neither may be nil.
		if monitor.Conditions == nil {
			t.Error("conditions should default to an empty array")
		}
		if monitor.NotificationIDList == nil {
			t.Error("notification list should default to an empty map")
		}
	})

	t.Run("retry interval mirrors the check interval", func(t *testing.T) {
		t.Parallel()

		monitor := kuma.Monitor{Name: "web", Type: "http", Interval: 300}
		kuma.NormalizeMonitor(&monitor)

		if monitor.RetryInterval != 300 {
			t.Errorf("retry interval = %d, want 300", monitor.RetryInterval)
		}

		// An explicit value is left alone.
		explicit := kuma.Monitor{Name: "web", Type: "http", Interval: 300, RetryInterval: 30}
		kuma.NormalizeMonitor(&explicit)
		if explicit.RetryInterval != 30 {
			t.Errorf("explicit retry interval = %d, want 30", explicit.RetryInterval)
		}
	})

	t.Run("push monitors get a generated token", func(t *testing.T) {
		t.Parallel()

		monitor := kuma.Monitor{Name: "backup", Type: "push", Interval: 3600}
		kuma.NormalizeMonitor(&monitor)

		if monitor.PushToken == nil {
			t.Fatal("a push monitor needs a token; the server never makes one")
		}
		if len(*monitor.PushToken) != 32 {
			t.Errorf("token length = %d, want 32", len(*monitor.PushToken))
		}

		// Two monitors must not share a token.
		other := kuma.Monitor{Name: "other", Type: "push", Interval: 3600}
		kuma.NormalizeMonitor(&other)
		if *other.PushToken == *monitor.PushToken {
			t.Error("tokens must be unique per monitor")
		}

		// A token the user pinned is kept.
		pinned := "pinned-token-value"
		fixed := kuma.Monitor{Name: "fixed", Type: "push", PushToken: &pinned}
		kuma.NormalizeMonitor(&fixed)
		if *fixed.PushToken != pinned {
			t.Errorf("token = %q, want the pinned one", *fixed.PushToken)
		}
	})
}

// TestHappyNormalizeMaintenance covers the three fields the server dereferences
// or requires, across the strategies that use them.
func TestHappyNormalizeMaintenance(t *testing.T) {
	t.Parallel()

	t.Run("manual strategy still needs a date range", func(t *testing.T) {
		t.Parallel()

		maintenance := kuma.Maintenance{Title: "w", Description: "d", Strategy: "manual"}
		kuma.NormalizeMaintenance(&maintenance)

		// jsonToBean indexes dateRange[0] and [1] whatever the strategy is.
		if len(maintenance.DateRange) != 2 {
			t.Fatalf("date range = %v, want two entries", maintenance.DateRange)
		}
		if maintenance.DateRange[0] != nil || maintenance.DateRange[1] != nil {
			t.Error("both ends should be null when no dates were given")
		}
	})

	t.Run("active defaults to true because the column is NOT NULL", func(t *testing.T) {
		t.Parallel()

		maintenance := kuma.Maintenance{Title: "w", Description: "d", Strategy: "manual"}
		kuma.NormalizeMaintenance(&maintenance)

		if maintenance.Active == nil || !maintenance.Active.Value() {
			t.Errorf("active = %v, want true", maintenance.Active)
		}

		// An explicit false is preserved.
		paused := kuma.Maintenance{Title: "w", Description: "d", Strategy: "manual", Active: kuma.BoolPtr(false)}
		kuma.NormalizeMaintenance(&paused)
		if paused.Active.Value() {
			t.Error("an explicit false must survive")
		}
	})

	t.Run("recurring strategies get a time range and a timezone", func(t *testing.T) {
		t.Parallel()

		maintenance := kuma.Maintenance{Title: "w", Description: "d", Strategy: "recurring-weekday"}
		kuma.NormalizeMaintenance(&maintenance)

		if len(maintenance.TimeRange) != 2 {
			t.Errorf("time range = %v, want two entries", maintenance.TimeRange)
		}
		if maintenance.TimezoneOption == nil || *maintenance.TimezoneOption != "SAME_AS_SERVER" {
			t.Errorf("timezone = %v, want SAME_AS_SERVER", maintenance.TimezoneOption)
		}
		// Empty slices, not nil: the server JSON-parses both.
		if maintenance.Weekdays == nil || maintenance.DaysOfMonth == nil {
			t.Error("weekdays and daysOfMonth should be empty slices, not nil")
		}
	})
}

// TestHappyAcknowledgements covers the three acknowledgement shapes the server
// uses for a successful call.
func TestHappyAcknowledgements(t *testing.T) {
	t.Parallel()

	t.Run("create returns the new ID", func(t *testing.T) {
		t.Parallel()

		var ack struct {
			OK        bool `json:"ok"`
			MonitorID int  `json:"monitorID"`
		}
		if err := json.Unmarshal([]byte(`{"ok":true,"msg":"successAdded","monitorID":42}`), &ack); err != nil {
			t.Fatalf("decoding: %v", err)
		}
		if !ack.OK || ack.MonitorID != 42 {
			t.Errorf("got %+v", ack)
		}
	})

	t.Run("list getters acknowledge with ok only", func(t *testing.T) {
		t.Parallel()

		// getMonitorList and friends answer {ok:true} and push the payload
		// separately, so an empty-looking ack is still success.
		var ack struct {
			OK bool `json:"ok"`
		}
		if err := json.Unmarshal([]byte(`{"ok":true}`), &ack); err != nil {
			t.Fatalf("decoding: %v", err)
		}
		if !ack.OK {
			t.Error("ok should be true")
		}
	})

	t.Run("needSetup answers with a bare boolean", func(t *testing.T) {
		t.Parallel()

		// This handler skips the {ok: ...} envelope entirely.
		var need bool
		if err := json.Unmarshal([]byte(`true`), &need); err != nil {
			t.Fatalf("decoding: %v", err)
		}
		if !need {
			t.Error("needSetup should be true")
		}
	})
}
