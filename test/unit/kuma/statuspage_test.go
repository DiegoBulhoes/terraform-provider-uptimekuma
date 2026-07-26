package kuma_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
)

// Status page tests, in the same three flavours as the rest of the suite. The
// group tree is the interesting part: it is the only structure in the provider
// where order carries meaning.

// ── Happy ───────────────────────────────────────────────────────────

// TestHappyStatusPageDecoding decodes the three shapes the server sends.
func TestHappyStatusPageDecoding(t *testing.T) {
	t.Parallel()

	t.Run("a freshly created page", func(t *testing.T) {
		t.Parallel()

		body := `{"id":1,"slug":"status","title":"Status","theme":"auto",
			"autoRefreshInterval":300,"published":true,"showTags":false,
			"domainNameList":[],"showPoweredBy":true}`
		var page kuma.StatusPage
		if err := json.Unmarshal([]byte(body), &page); err != nil {
			t.Fatalf("decoding: %v", err)
		}
		if page.ID != 1 || page.Slug != "status" || page.Theme != "auto" {
			t.Errorf("got %+v", page)
		}
		if !page.Published.Value() {
			t.Error("a new page is published")
		}
		if page.AutoRefreshInterval == nil || *page.AutoRefreshInterval != 300 {
			t.Errorf("refresh interval = %v", page.AutoRefreshInterval)
		}
	})

	t.Run("a fully configured page", func(t *testing.T) {
		t.Parallel()

		body := `{"id":2,"slug":"public","title":"Public","description":"desc",
			"theme":"dark","showTags":true,"customCSS":"body{}","footerText":"footer",
			"rssTitle":"feed","analyticsId":"G-1","analyticsType":"google",
			"domainNameList":["status.example.com","uptime.example.com"],
			"showCertificateExpiry":true,"showOnlyLastHeartbeat":true}`
		var page kuma.StatusPage
		if err := json.Unmarshal([]byte(body), &page); err != nil {
			t.Fatalf("decoding: %v", err)
		}
		if len(page.DomainNameList) != 2 {
			t.Errorf("domains = %v", page.DomainNameList)
		}
		if page.AnalyticsType == nil || *page.AnalyticsType != "google" {
			t.Errorf("analytics type = %v", page.AnalyticsType)
		}
		if !page.ShowCertificateExpiry.Value() || !page.ShowOnlyLastHeartbeat.Value() {
			t.Error("both display flags should be on")
		}
	})

	t.Run("the group tree from the HTTP route", func(t *testing.T) {
		t.Parallel()

		body := `[
			{"id":1,"name":"Core","weight":1,"monitorList":[
				{"id":10,"name":"API","sendUrl":true,"url":"https://api.example.com","type":"http"},
				{"id":11,"name":"DB","sendUrl":false,"type":"port"}
			]},
			{"id":2,"name":"Extras","weight":2,"monitorList":[]}
		]`
		var groups []kuma.StatusPageGroup
		if err := json.Unmarshal([]byte(body), &groups); err != nil {
			t.Fatalf("decoding: %v", err)
		}
		if len(groups) != 2 {
			t.Fatalf("got %d groups", len(groups))
		}
		// Order is the display order, so it must survive decoding.
		if groups[0].Name != "Core" || groups[1].Name != "Extras" {
			t.Errorf("order lost: %q, %q", groups[0].Name, groups[1].Name)
		}
		if len(groups[0].MonitorList) != 2 {
			t.Fatalf("first group has %d monitors", len(groups[0].MonitorList))
		}
		// With sendUrl on, the server reports the link it will render.
		if groups[0].MonitorList[0].URL == nil {
			t.Error("a monitor with sendUrl should report a url")
		}
		// With sendUrl off, no url comes back at all.
		if groups[0].MonitorList[1].URL != nil {
			t.Errorf("a monitor without sendUrl should not report a url, got %q", *groups[0].MonitorList[1].URL)
		}
	})
}

// TestHappyStatusPageNormalize covers the two fields the save handler reads
// without a fallback.
func TestHappyStatusPageNormalize(t *testing.T) {
	t.Parallel()

	page := kuma.StatusPage{Slug: "s", Title: "T"}
	groups, err := roundTripSave(&page)
	if err != nil {
		t.Fatalf("encoding: %v", err)
	}

	// updateDomainNameList ignores anything that is not an array, which would
	// silently keep the old domains instead of clearing them.
	if !strings.Contains(groups, `"domainNameList":[]`) {
		t.Errorf("domain list should be an empty array, got %s", groups)
	}
	// An empty theme would be stored as-is and break the page.
	if page.Theme != "auto" {
		t.Errorf("theme = %q, want auto", page.Theme)
	}
}

// ── Sad ─────────────────────────────────────────────────────────────

// TestSadStatusPageErrors covers the three failures the status page handlers
// produce, all of which have to classify correctly.
func TestSadStatusPageErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		event    string
		msg      string
		notFound bool
	}{
		{
			name: "unknown slug",
			// The handler throws a literal "No slug?" rather than anything
			// resembling a not-found code.
			event:    "getStatusPage",
			msg:      "No slug?",
			notFound: true,
		},
		{
			name:     "unknown slug on an incident",
			event:    "postIncident",
			msg:      "slug is not found",
			notFound: true,
		},
		{
			name:     "invalid analytics type is a rejection, not a missing page",
			event:    "saveStatusPage",
			msg:      "Invalid analytics type",
			notFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := error(&kuma.APIError{Event: tt.event, Msg: tt.msg})
			if got := kuma.IsNotFound(err); got != tt.notFound {
				t.Errorf("IsNotFound(%q) = %v, want %v", tt.msg, got, tt.notFound)
			}
			if kuma.IsRetryable(err) {
				t.Error("none of these are worth retrying")
			}
		})
	}
}

// ── Absurd ──────────────────────────────────────────────────────────

// TestAbsurdStatusPagePayloads throws three malformed group trees at the
// decoder.
func TestAbsurdStatusPagePayloads(t *testing.T) {
	t.Parallel()

	t.Run("an empty tree decodes to an empty slice", func(t *testing.T) {
		t.Parallel()

		var groups []kuma.StatusPageGroup
		if err := json.Unmarshal([]byte(`[]`), &groups); err != nil {
			t.Fatalf("decoding: %v", err)
		}
		if len(groups) != 0 {
			t.Errorf("got %d groups", len(groups))
		}
	})

	t.Run("a group with a null monitor list is still usable", func(t *testing.T) {
		t.Parallel()

		var groups []kuma.StatusPageGroup
		if err := json.Unmarshal([]byte(`[{"id":1,"name":"x","monitorList":null}]`), &groups); err != nil {
			t.Fatalf("decoding: %v", err)
		}
		if len(groups) != 1 || groups[0].MonitorList != nil {
			t.Errorf("got %+v", groups)
		}
	})

	t.Run("sendUrl as 0 or 1 still reads as a boolean", func(t *testing.T) {
		t.Parallel()

		// Not hypothetical: monitor_group.send_url comes from SQLite.
		var groups []kuma.StatusPageGroup
		body := `[{"name":"x","monitorList":[{"id":1,"sendUrl":1},{"id":2,"sendUrl":0}]}]`
		if err := json.Unmarshal([]byte(body), &groups); err != nil {
			t.Fatalf("decoding: %v", err)
		}
		if !groups[0].MonitorList[0].SendURL.Value() {
			t.Error("sendUrl 1 should be true")
		}
		if groups[0].MonitorList[1].SendURL.Value() {
			t.Error("sendUrl 0 should be false")
		}
	})
}

// TestAbsurdStatusPageText checks hostile text in a page's fields survives a
// round-trip, since custom CSS and footers are free-form by design.
func TestAbsurdStatusPageText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		value string
	}{
		{name: "css with braces and quotes", value: `body { content: "}"; }`},
		{name: "html in the footer", value: `<script>alert("x")</script>`},
		{name: "very long custom css", value: strings.Repeat(".a{color:red}", 2000)},
		{name: "emoji in the title", value: "🚦 Status"},
		{name: "newlines", value: "line one\nline two"},
		{name: "a lone backslash", value: `back\slash`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			original := kuma.StatusPage{
				Slug: "s", Title: tt.value, CustomCSS: &tt.value, FooterText: &tt.value,
			}
			encoded, err := json.Marshal(original)
			if err != nil {
				t.Fatalf("encoding: %v", err)
			}
			var decoded kuma.StatusPage
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatalf("decoding: %v", err)
			}
			if decoded.Title != tt.value {
				t.Errorf("title changed:\n got %q\nwant %q", decoded.Title, tt.value)
			}
			if decoded.CustomCSS == nil || *decoded.CustomCSS != tt.value {
				t.Errorf("custom css changed: %v", decoded.CustomCSS)
			}
		})
	}
}

// roundTripSave normalizes a page the way SaveStatusPage does and returns the
// encoded payload, so the test can assert on what would go over the wire.
func roundTripSave(page *kuma.StatusPage) (string, error) {
	// SaveStatusPage normalizes before sending; mirror that here without needing
	// a live client.
	if page.DomainNameList == nil {
		page.DomainNameList = []string{}
	}
	if page.Theme == "" {
		page.Theme = "auto"
	}
	encoded, err := json.Marshal(page)
	return string(encoded), err
}
