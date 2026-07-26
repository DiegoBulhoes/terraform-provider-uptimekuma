package resource_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/maintenance"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/notification"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/settings"
)

// Absurd-input tests for the resource side. The settings and notification
// resources take raw JSON from the user, which makes them the widest opening in
// the provider: whatever arrives has to be either accepted intact or rejected
// with a message that says what to fix.

// TestAbsurdNotificationSettings throws three hostile settings documents at the
// payload builder.
func TestAbsurdNotificationSettings(t *testing.T) {
	t.Parallel()

	t.Run("deeply nested JSON is passed through untouched", func(t *testing.T) {
		t.Parallel()

		// Some providers really do take nested objects, so nesting must survive
		// rather than be flattened or dropped.
		nested := `{"webhookAdditionalHeaders":{"a":{"b":{"c":["d","e"]}}}}`
		payload, err := notification.BuildPayload("c", "webhook", false, false, nested)
		if err != nil {
			t.Fatalf("BuildPayload: %v", err)
		}

		encoded, err := json.Marshal(payload["webhookAdditionalHeaders"])
		if err != nil {
			t.Fatalf("re-encoding: %v", err)
		}
		if !strings.Contains(string(encoded), `"c":["d","e"]`) {
			t.Errorf("nesting was mangled: %s", encoded)
		}
	})

	t.Run("every reserved key is refused, not just the obvious one", func(t *testing.T) {
		t.Parallel()

		for _, key := range []string{"id", "name", "type", "isDefault", "applyExisting", "active", "userId"} {
			body := `{"` + key + `":"x"}`
			if _, err := notification.BuildPayload("c", "webhook", false, false, body); err == nil {
				t.Errorf("settings containing %q should be refused", key)
			}
		}
	})

	t.Run("absurd values inside settings are the server's problem, not ours", func(t *testing.T) {
		t.Parallel()

		// The provider does not validate provider-specific fields — there are
		// about a hundred providers and their schemas change upstream. Anything
		// syntactically valid must reach the server, which decides.
		absurd := []string{
			`{"webhookURL":""}`,
			`{"webhookURL":null}`,
			`{"smtpPort":-1}`,
			`{"unknownFieldFromTheFuture":true}`,
			`{"veryLong":"` + strings.Repeat("x", 5000) + `"}`,
			`{"emoji":"🔥"}`,
		}
		for _, body := range absurd {
			if _, err := notification.BuildPayload("c", "webhook", false, false, body); err != nil {
				t.Errorf("%.40s… should be accepted and left to the server: %v", body, err)
			}
		}
	})
}

// TestAbsurdClockTime feeds three inputs that look almost like times.
func TestAbsurdClockTime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "trailing text after a valid time", input: "02:00 and then some", wantErr: true},
		{name: "negative hour", input: "-2:00", wantErr: true},
		{name: "seconds included", input: "02:00:00", wantErr: true},
		{name: "empty string", input: "", wantErr: true},
		{name: "only a colon", input: ":", wantErr: true},
		{name: "single digits are accepted and padded", input: "2:5", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			part, err := maintenance.ParseClockTime(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("%q should be rejected, got %v", tt.input, part)
				}
				return
			}
			if err != nil {
				t.Fatalf("%q should be accepted: %v", tt.input, err)
			}
			// "2:5" means 02:05, and must render canonically.
			if got := maintenance.FormatClockTime(part); got != "02:05" {
				t.Errorf("rendered %q, want 02:05", got)
			}
		})
	}
}

// TestAbsurdSettingsDocuments covers three settings documents that are valid JSON
// but nonsense as configuration.
func TestAbsurdSettingsDocuments(t *testing.T) {
	t.Parallel()

	t.Run("unknown keys are accepted, because the key set is version-specific", func(t *testing.T) {
		t.Parallel()

		// Refusing unknown keys would break the provider every time upstream adds
		// a setting; the server ignores what it does not recognize.
		parsed, err := settings.ParseManaged(`{"someSettingFromKuma3":"value"}`)
		if err != nil {
			t.Fatalf("ParseManaged: %v", err)
		}
		if parsed["someSettingFromKuma3"] != "value" {
			t.Errorf("unknown key was dropped: %v", parsed)
		}
	})

	t.Run("wildly wrong value types still parse", func(t *testing.T) {
		t.Parallel()

		// keepDataPeriodDays wants a number; the server rejects the rest. Failing
		// here instead would only duplicate its validation, badly.
		for _, body := range []string{
			`{"keepDataPeriodDays":"not a number"}`,
			`{"keepDataPeriodDays":[1,2,3]}`,
			`{"keepDataPeriodDays":{"nested":true}}`,
			`{"keepDataPeriodDays":-99999}`,
		} {
			if _, err := settings.ParseManaged(body); err != nil {
				t.Errorf("%s should parse and let the server judge: %v", body, err)
			}
		}
	})

	t.Run("malformed documents are rejected", func(t *testing.T) {
		t.Parallel()

		for _, body := range []string{
			``,
			`{`,
			`null`,
			`42`,
			`[{"keepDataPeriodDays":1}]`,
			`{"a":1}{"b":2}`,
		} {
			if _, err := settings.ParseManaged(body); err == nil {
				t.Errorf("%q should be rejected as a settings document", body)
			}
		}
	})
}

// TestAbsurdPayloadNameAndType checks that absurd names and types are forwarded
// intact. Uptime Kuma keeps adding notification providers, so the provider does
// not police the type — but it must not corrupt it either.
func TestAbsurdPayloadNameAndType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
	}{
		{name: "empty", value: ""},
		{name: "only spaces", value: "   "},
		{name: "very long", value: strings.Repeat("n", 5000)},
		{name: "emoji", value: "🚨 alerts"},
		{name: "quotes and backslashes", value: `he said "hi"\ then left`},
		{name: "newlines", value: "line one\nline two"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			payload, err := notification.BuildPayload(tt.value, tt.value, false, false, `{"webhookURL":"https://x"}`)
			if err != nil {
				t.Fatalf("BuildPayload: %v", err)
			}
			if payload["name"] != tt.value {
				t.Errorf("name changed: %q", payload["name"])
			}
			if payload["type"] != tt.value {
				t.Errorf("type changed: %q", payload["type"])
			}
		})
	}
}

// TestAbsurdEmptySettings checks the three ways of saying "no settings", all of
// which must still produce a usable payload.
func TestAbsurdEmptySettings(t *testing.T) {
	t.Parallel()

	for _, body := range []string{"", "{}"} {
		payload, err := notification.BuildPayload("c", "webhook", false, false, body)
		if err != nil {
			t.Fatalf("%q should be accepted: %v", body, err)
		}
		// Even with nothing configured, the four promoted keys must be there or
		// the server cannot store the channel.
		for _, key := range []string{"name", "type", "isDefault", "applyExisting"} {
			if _, ok := payload[key]; !ok {
				t.Errorf("%q is missing for settings %q", key, body)
			}
		}
		if len(payload) != 4 {
			t.Errorf("expected only the promoted keys, got %v", payload)
		}
	}
}
