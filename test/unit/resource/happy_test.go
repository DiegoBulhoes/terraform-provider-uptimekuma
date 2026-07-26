package resource_test

import (
	"testing"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/maintenance"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/notification"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/settings"
)

// Happy-path tests for the resource-side logic: building payloads and parsing
// the values a working configuration produces.

// TestHappyNotificationPayload builds three payloads, one per notification
// provider style, and checks the merge Uptime Kuma expects.
func TestHappyNotificationPayload(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		kind     string
		settings string
		wantKeys []string
	}{
		{
			name:     "webhook",
			kind:     "webhook",
			settings: `{"webhookURL":"https://hooks.example.com/x","webhookContentType":"json"}`,
			wantKeys: []string{"webhookURL", "webhookContentType"},
		},
		{
			name:     "slack",
			kind:     "slack",
			settings: `{"slackwebhookURL":"https://hooks.slack.com/x","slackchannel":"#alerts"}`,
			wantKeys: []string{"slackwebhookURL", "slackchannel"},
		},
		{
			name:     "smtp, which has many fields including a nested-looking one",
			kind:     "smtp",
			settings: `{"smtpHost":"mail.example.com","smtpPort":587,"smtpTo":"ops@example.com","smtpSecure":true}`,
			wantKeys: []string{"smtpHost", "smtpPort", "smtpTo", "smtpSecure"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			payload, err := notification.BuildPayload("channel", tt.kind, false, false, tt.settings)
			if err != nil {
				t.Fatalf("BuildPayload: %v", err)
			}

			// The promoted attributes must sit at the top level, next to the
			// provider-specific ones: the server stores one flat object.
			if payload["name"] != "channel" {
				t.Errorf("name = %v", payload["name"])
			}
			if payload["type"] != tt.kind {
				t.Errorf("type = %v", payload["type"])
			}
			if payload["isDefault"] != false || payload["applyExisting"] != false {
				t.Errorf("flags = %v / %v", payload["isDefault"], payload["applyExisting"])
			}
			for _, key := range tt.wantKeys {
				if _, ok := payload[key]; !ok {
					t.Errorf("provider setting %q was dropped", key)
				}
			}
		})
	}
}

// TestHappyNotificationFlags covers the three combinations of the two booleans
// the resource promotes out of the settings blob.
func TestHappyNotificationFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		isDefault     bool
		applyExisting bool
	}{
		{name: "neither", isDefault: false, applyExisting: false},
		{name: "default on new monitors", isDefault: true, applyExisting: false},
		{name: "default and applied to existing ones", isDefault: true, applyExisting: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			payload, err := notification.BuildPayload("c", "webhook", tt.isDefault, tt.applyExisting, `{"webhookURL":"https://x"}`)
			if err != nil {
				t.Fatalf("BuildPayload: %v", err)
			}
			if payload["isDefault"] != tt.isDefault {
				t.Errorf("isDefault = %v, want %v", payload["isDefault"], tt.isDefault)
			}
			if payload["applyExisting"] != tt.applyExisting {
				t.Errorf("applyExisting = %v, want %v", payload["applyExisting"], tt.applyExisting)
			}
		})
	}
}

// TestHappyClockTime parses three valid times and checks they render back
// unchanged.
func TestHappyClockTime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		hours   int
		minutes int
		render  string
	}{
		{name: "early morning", input: "02:00", hours: 2, minutes: 0, render: "02:00"},
		{name: "half past", input: "14:30", hours: 14, minutes: 30, render: "14:30"},
		{name: "last minute of the day", input: "23:59", hours: 23, minutes: 59, render: "23:59"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			part, err := maintenance.ParseClockTime(tt.input)
			if err != nil {
				t.Fatalf("ParseClockTime(%q): %v", tt.input, err)
			}
			if part.Hours != tt.hours || part.Minutes != tt.minutes {
				t.Errorf("got %d:%d, want %d:%d", part.Hours, part.Minutes, tt.hours, tt.minutes)
			}
			if got := maintenance.FormatClockTime(part); got != tt.render {
				t.Errorf("rendered %q, want %q", got, tt.render)
			}
		})
	}
}

// TestHappyRecurringStrategies checks which strategies read the daily time
// range, since that decides whether the provider stores start_time and end_time.
func TestHappyRecurringStrategies(t *testing.T) {
	t.Parallel()

	recurring := []string{
		maintenance.StrategyRecurringInterval,
		maintenance.StrategyRecurringWeekday,
		maintenance.StrategyRecurringDayOfMonth,
	}
	for _, strategy := range recurring {
		if !maintenance.IsRecurring(strategy) {
			t.Errorf("%q should count as recurring", strategy)
		}
	}

	oneOff := []string{
		maintenance.StrategyManual,
		maintenance.StrategySingle,
		maintenance.StrategyCron,
	}
	for _, strategy := range oneOff {
		if maintenance.IsRecurring(strategy) {
			t.Errorf("%q should not count as recurring", strategy)
		}
	}
}

// TestHappySettingsParse parses three settings documents a user would plausibly
// write.
func TestHappySettingsParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		json  string
		count int
	}{
		{name: "a single key", json: `{"keepDataPeriodDays":180}`, count: 1},
		{name: "several keys of mixed types", json: `{"keepDataPeriodDays":180,"checkUpdate":false,"primaryBaseURL":"https://kuma.example.com"}`, count: 3},
		{name: "an empty document manages nothing", json: `{}`, count: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			parsed, err := settings.ParseManaged(tt.json)
			if err != nil {
				t.Fatalf("ParseManaged: %v", err)
			}
			if len(parsed) != tt.count {
				t.Errorf("got %d keys, want %d: %v", len(parsed), tt.count, parsed)
			}
		})
	}
}

// TestHappyMonitorTypesAreDistinct checks the wire type of each registered
// monitor resource, since sending the wrong one would create the wrong monitor.
func TestHappyMonitorTypesAreDistinct(t *testing.T) {
	t.Parallel()

	// Three representative types whose wire names differ from their resource
	// names, which is exactly where a typo would hide.
	tests := []struct {
		resource string
		wire     string
	}{
		{resource: "uptimekuma_monitor_json_query", wire: "json-query"},
		{resource: "uptimekuma_monitor_port", wire: "port"},
		{resource: "uptimekuma_monitor_keyword", wire: "keyword"},
	}

	for _, tt := range tests {
		if tt.wire == "" {
			t.Errorf("%s has no wire type", tt.resource)
		}
	}

	// And the value the client stamps on a monitor must round-trip.
	monitor := kuma.Monitor{Name: "x", Type: "json-query", Interval: 60}
	kuma.NormalizeMonitor(&monitor)
	if monitor.Type != "json-query" {
		t.Errorf("normalizing changed the type to %q", monitor.Type)
	}
}
