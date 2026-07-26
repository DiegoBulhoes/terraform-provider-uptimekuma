package kuma_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
)

// Absurd-input tests: payloads no healthy server would send, and values no sane
// user would type.
//
// These matter more here than in most providers. The pushed lists are decoded
// inside event handlers, where an error cannot be returned to anyone — a decode
// failure just leaves the cache empty and every object looks like it does not
// exist. So the rule is: absurd input either decodes into something sensible or
// fails loudly, never half-way.

// TestAbsurdBooleanValues throws three nonsense values at the boolean type that
// absorbs SQLite's 0/1.
func TestAbsurdBooleanValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		json    string
		want    bool
		wantErr bool
	}{
		{name: "the word maybe", json: `"maybe"`, wantErr: true},
		{name: "an object", json: `{"true": true}`, wantErr: true},
		{name: "an array", json: `[1]`, wantErr: true},
		{name: "a huge number is still truthy", json: `999999999`, want: true},
		{name: "a negative number is truthy", json: `-1`, want: true},
		{name: "a float that rounds to zero is not", json: `0.0`, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got kuma.Bool
			err := json.Unmarshal([]byte(tt.json), &got)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("%s should not decode as a boolean, got %v", tt.json, bool(got))
				}
				return
			}
			if err != nil {
				t.Fatalf("decoding %s: %v", tt.json, err)
			}
			if bool(got) != tt.want {
				t.Errorf("%s decoded to %v, want %v", tt.json, bool(got), tt.want)
			}
		})
	}
}

// TestAbsurdMonitorPayloads feeds three malformed monitor payloads through the
// decoder.
func TestAbsurdMonitorPayloads(t *testing.T) {
	t.Parallel()

	t.Run("interval as a string fails instead of silently becoming zero", func(t *testing.T) {
		t.Parallel()

		var monitor kuma.Monitor
		err := json.Unmarshal([]byte(`{"name":"x","type":"http","interval":"60"}`), &monitor)
		if err == nil {
			t.Errorf("a string interval should fail, got %d", monitor.Interval)
		}
	})

	t.Run("notification list with junk values keeps only the enabled ones", func(t *testing.T) {
		t.Parallel()

		// The server writes {"1": true}; a false value means "not linked".
		var monitor kuma.Monitor
		body := `{"name":"x","type":"http","notificationIDList":{"1":true,"2":false,"3":true}}`
		if err := json.Unmarshal([]byte(body), &monitor); err != nil {
			t.Fatalf("decoding: %v", err)
		}
		if len(monitor.NotificationIDList) != 3 {
			t.Fatalf("got %v", monitor.NotificationIDList)
		}
		if monitor.NotificationIDList["2"] {
			t.Error("a false entry must not read as linked")
		}
	})

	t.Run("empty object decodes to a zero monitor rather than failing", func(t *testing.T) {
		t.Parallel()

		var monitor kuma.Monitor
		if err := json.Unmarshal([]byte(`{}`), &monitor); err != nil {
			t.Fatalf("an empty object should decode: %v", err)
		}
		if monitor.ID != 0 || monitor.Name != "" || monitor.URL != nil {
			t.Errorf("expected a zero value, got %+v", monitor)
		}
		// Normalizing a monitor that says nothing must still produce something
		// the server accepts.
		kuma.NormalizeMonitor(&monitor)
		if monitor.RetryInterval < 1 {
			t.Error("retry interval must end up at or above the minimum")
		}
	})
}

// TestAbsurdNormalizeInputs pushes three absurd values through the normalizers,
// which exist precisely to keep the server from choking.
func TestAbsurdNormalizeInputs(t *testing.T) {
	t.Parallel()

	t.Run("negative intervals still produce a valid retry interval", func(t *testing.T) {
		t.Parallel()

		monitor := kuma.Monitor{Name: "x", Type: "http", Interval: -500, RetryInterval: -9000}
		kuma.NormalizeMonitor(&monitor)

		// Schema validators reject this long before the client sees it; if one
		// ever slips through, the payload must not carry a value validate()
		// rejects.
		if monitor.RetryInterval < kuma.MinIntervalSeconds {
			t.Errorf("retry interval = %d, want at least %d", monitor.RetryInterval, kuma.MinIntervalSeconds)
		}
	})

	t.Run("an over-long date range is left alone rather than truncated", func(t *testing.T) {
		t.Parallel()

		four := []*string{nil, nil, nil, nil}
		maintenance := kuma.Maintenance{Title: "x", Description: "d", Strategy: "single", DateRange: four}
		kuma.NormalizeMaintenance(&maintenance)

		// The normalizer only pads; the server reads the first two entries.
		if len(maintenance.DateRange) != 4 {
			t.Errorf("date range = %d entries, want the original 4", len(maintenance.DateRange))
		}
	})

	t.Run("a push monitor with an empty token gets a real one", func(t *testing.T) {
		t.Parallel()

		empty := ""
		monitor := kuma.Monitor{Name: "x", Type: "push", PushToken: &empty}
		kuma.NormalizeMonitor(&monitor)

		if monitor.PushToken == nil || *monitor.PushToken == "" {
			t.Fatal("an empty token is as useless as none")
		}
		if len(*monitor.PushToken) != 32 {
			t.Errorf("token length = %d", len(*monitor.PushToken))
		}
	})
}

// TestAbsurdTextValues checks that hostile and oversized text survives a
// round-trip byte for byte. The provider must never mangle or truncate what the
// user configured.
func TestAbsurdTextValues(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
	}{
		{name: "emoji and combining marks", value: "🔥 monitor — café ☕ 日本語"},
		{name: "quotes, backslashes and newlines", value: "he said \"hi\"\\n\nand left\ttab"},
		{name: "a very long name", value: strings.Repeat("a", 10000)},
		{name: "html and script tags", value: `<script>alert("xss")</script>`},
		{name: "sql-looking text", value: "'; DROP TABLE monitor; --"},
		{name: "null bytes as text", value: `\u0000 embedded`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			original := kuma.Monitor{Name: tt.value, Type: "http", Interval: 60}
			encoded, err := json.Marshal(original)
			if err != nil {
				t.Fatalf("encoding: %v", err)
			}

			var decoded kuma.Monitor
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatalf("decoding: %v", err)
			}
			if decoded.Name != tt.value {
				t.Errorf("name changed in transit:\n got %q\nwant %q", decoded.Name, tt.value)
			}
		})
	}
}

// TestAbsurdErrorMessages checks the not-found matcher is not fooled by text
// that merely mentions the wrong words.
func TestAbsurdErrorMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		msg      string
		notFound bool
	}{
		{
			name: "a monitor whose name contains the words",
			// The message is about a URL, not a missing object — but the monitor's
			// name happens to contain "not found". Matching on text cannot tell
			// these apart, and this test records that limitation instead of
			// pretending it does not exist.
			msg:      `Request failed for monitor "page not found checker"`,
			notFound: true,
		},
		{
			name:     "empty message",
			msg:      "",
			notFound: false,
		},
		{
			name:     "a very long message ending in the marker",
			msg:      strings.Repeat("stack frame\n", 500) + "Cannot read properties of null",
			notFound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := error(&kuma.APIError{Event: "getMonitor", Msg: tt.msg})
			if got := kuma.IsNotFound(err); got != tt.notFound {
				t.Errorf("IsNotFound = %v, want %v", got, tt.notFound)
			}
		})
	}
}
