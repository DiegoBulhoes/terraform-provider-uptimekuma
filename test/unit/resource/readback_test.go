package resource_test

import (
	"encoding/json"
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

// Reading a fully-populated object back into state.
//
// The acceptance tests create simple objects, so the branches that handle
// optional fields — a maintenance window with weekdays and a duration, a monitor
// with tags and notifications, a status page with a group tree — are only walked
// when the server returns them. Feeding rich payloads through a mock is how those
// get exercised without inventing a dozen acceptance fixtures.

func ptr[T any](v T) *T { return &v }

func TestReadPopulatesFullState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		factory func() fwresource.Resource
		state   map[string]tftypes.Value
		expect  func(*mocks.MockKumaClient)
	}{
		{
			name:    "monitor with every shared attribute set",
			factory: monitor.NewHTTPResource,
			state:   map[string]tftypes.Value{"id": str("7")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().GetMonitor(gomock.Any(), 7).Return(&kuma.Monitor{
					ID: 7, Name: "web", Type: "http",
					Description:         ptr("a description"),
					URL:                 ptr("https://example.com"),
					Method:              ptr("POST"),
					Body:                ptr(`{"a":1}`),
					Headers:             ptr(`{"X-A":"b"}`),
					Interval:            120,
					RetryInterval:       60,
					ResendInterval:      3,
					MaxRetries:          2,
					Timeout:             ptr(48.0),
					Weight:              ptr(2000),
					Parent:              ptr(3),
					Active:              kuma.BoolPtr(true),
					UpsideDown:          kuma.BoolPtr(true),
					IgnoreTLS:           kuma.BoolPtr(true),
					MaxRedirects:        ptr(5),
					ProxyID:             ptr(4),
					AcceptedStatusCodes: []string{"200-299", "404"},
					// The set is an object keyed by stringified ID, and a false
					// entry means "not linked".
					NotificationIDList: map[string]bool{"1": true, "2": false, "3": true},
					Tags: []kuma.MonitorTag{
						{TagID: 10, Value: ptr("production"), Name: "env", Color: "#fff"},
						{TagID: 11, Value: nil, Name: "team", Color: "#000"},
					},
					HTTPBodyEncoding:  ptr("json"),
					AuthMethod:        ptr("basic"),
					BasicAuthUser:     ptr("user"),
					BasicAuthPass:     ptr("pass"),
					SaveResponse:      kuma.BoolPtr(true),
					SaveErrorResponse: kuma.BoolPtr(false),
					ResponseMaxLength: ptr(8192),
				}, nil)
			},
		},
		{
			name:    "monitor with everything optional absent",
			factory: monitor.NewPingResource,
			state:   map[string]tftypes.Value{"id": str("8")},
			expect: func(c *mocks.MockKumaClient) {
				// The other side of every branch: nothing set, so the model has to
				// end up with nulls rather than zero values.
				c.EXPECT().GetMonitor(gomock.Any(), 8).Return(&kuma.Monitor{
					ID: 8, Name: "ping", Type: "ping", Interval: 60, RetryInterval: 60,
					Hostname: ptr("127.0.0.1"),
					// Timeout 0 means "no timeout configured" for types that do
					// not use one, and must read back as null.
					Timeout: ptr(0.0),
				}, nil)
			},
		},
		{
			name:    "keyword monitor",
			factory: monitor.NewKeywordResource,
			state:   map[string]tftypes.Value{"id": str("9")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().GetMonitor(gomock.Any(), 9).Return(&kuma.Monitor{
					ID: 9, Name: "kw", Type: "keyword", Interval: 60, RetryInterval: 60,
					URL:                         ptr("https://example.com"),
					Keyword:                     ptr("Example"),
					InvertKeyword:               kuma.BoolPtr(true),
					RetryOnlyOnStatusCodeFailed: kuma.BoolPtr(true),
				}, nil)
			},
		},
		{
			name:    "json query monitor",
			factory: monitor.NewJSONQueryResource,
			state:   map[string]tftypes.Value{"id": str("10")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().GetMonitor(gomock.Any(), 10).Return(&kuma.Monitor{
					ID: 10, Name: "jq", Type: "json-query", Interval: 60, RetryInterval: 60,
					URL:              ptr("https://example.com"),
					JSONPath:         ptr("status"),
					JSONPathOperator: ptr("=="),
					ExpectedValue:    ptr("ok"),
				}, nil)
			},
		},
		{
			name:    "dns monitor reports its last result",
			factory: monitor.NewDNSResource,
			state:   map[string]tftypes.Value{"id": str("11")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().GetMonitor(gomock.Any(), 11).Return(&kuma.Monitor{
					ID: 11, Name: "dns", Type: "dns", Interval: 60, RetryInterval: 60,
					Hostname:         ptr("example.com"),
					DNSResolveServer: ptr("1.1.1.1"),
					DNSResolveType:   ptr("A"),
					Port:             ptr(53),
					DNSLastResult:    ptr("93.184.216.34"),
				}, nil)
			},
		},
		{
			name:    "push monitor derives its url from the token",
			factory: monitor.NewPushResource,
			state:   map[string]tftypes.Value{"id": str("12")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().GetMonitor(gomock.Any(), 12).Return(&kuma.Monitor{
					ID: 12, Name: "push", Type: "push", Interval: 3600, RetryInterval: 3600,
					PushToken: ptr("abc123"),
				}, nil)
			},
		},
		{
			name:    "group monitor reports its children",
			factory: monitor.NewGroupResource,
			state:   map[string]tftypes.Value{"id": str("13")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().GetMonitor(gomock.Any(), 13).Return(&kuma.Monitor{
					ID: 13, Name: "group", Type: "group", Interval: 60, RetryInterval: 60,
					ChildrenIDs: []int{14, 15},
				}, nil)
			},
		},
		{
			name:    "port monitor",
			factory: monitor.NewPortResource,
			state:   map[string]tftypes.Value{"id": str("16")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().GetMonitor(gomock.Any(), 16).Return(&kuma.Monitor{
					ID: 16, Name: "port", Type: "port", Interval: 60, RetryInterval: 60,
					Hostname: ptr("db.internal"), Port: ptr(5432), IPFamily: ptr("ipv4"),
				}, nil)
			},
		},
		{
			name:    "docker monitor",
			factory: monitor.NewDockerResource,
			state:   map[string]tftypes.Value{"id": str("17")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().GetMonitor(gomock.Any(), 17).Return(&kuma.Monitor{
					ID: 17, Name: "docker", Type: "docker", Interval: 60, RetryInterval: 60,
					DockerContainer: ptr("redis"), DockerHost: ptr(2),
				}, nil)
			},
		},
		{
			name:    "tag",
			factory: tag.New,
			state:   map[string]tftypes.Value{"id": str("3")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().GetTag(gomock.Any(), 3).Return(&kuma.Tag{ID: 3, Name: "env", Color: "#4B5563"}, nil)
			},
		},
		{
			name:    "notification splits the config back apart",
			factory: notification.New,
			state:   map[string]tftypes.Value{"id": str("5")},
			expect: func(c *mocks.MockKumaClient) {
				// The whole channel is one JSON string; the promoted attributes have
				// to come out of it and the rest stay in `settings`.
				config, _ := json.Marshal(map[string]any{
					"id": 5, "name": "hook", "type": "webhook", "isDefault": true,
					"active": true, "userId": 1,
					"webhookURL": "https://example.com/hook", "webhookContentType": "json",
				})
				c.EXPECT().GetNotification(gomock.Any(), 5).Return(&kuma.Notification{
					ID: 5, Name: "hook", Active: kuma.Bool(true), IsDefault: kuma.Bool(true),
					Config: string(config),
				}, nil)
			},
		},
		{
			name:    "notification with no settings at all",
			factory: notification.New,
			state:   map[string]tftypes.Value{"id": str("6")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().GetNotification(gomock.Any(), 6).Return(&kuma.Notification{
					ID: 6, Name: "bare", Config: `{"name":"bare","type":"webhook"}`,
				}, nil)
			},
		},
		{
			name:    "proxy with credentials",
			factory: proxy.New,
			state:   map[string]tftypes.Value{"id": str("2")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().GetProxy(gomock.Any(), 2).Return(&kuma.Proxy{
					ID: 2, Protocol: "socks5", Host: "proxy.internal", Port: 1080,
					Auth: kuma.Bool(true), Username: ptr("u"), Password: ptr("p"),
					Active: kuma.Bool(true), Default: kuma.Bool(true),
				}, nil)
			},
		},
		{
			name:    "proxy without credentials",
			factory: proxy.New,
			state:   map[string]tftypes.Value{"id": str("3")},
			expect: func(c *mocks.MockKumaClient) {
				// Empty strings, which is how the server stores "unset", must read
				// back as null.
				c.EXPECT().GetProxy(gomock.Any(), 3).Return(&kuma.Proxy{
					ID: 3, Protocol: "http", Host: "p", Port: 3128,
					Username: ptr(""), Password: ptr(""),
				}, nil)
			},
		},
		{
			name:    "docker host",
			factory: dockerhost.New,
			state:   map[string]tftypes.Value{"id": str("1")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().GetDockerHost(gomock.Any(), 1).Return(&kuma.DockerHost{
					ID: 1, Name: "local", DockerType: "socket", DockerDaemon: "/var/run/docker.sock",
				}, nil)
			},
		},
		{
			name:    "remote browser",
			factory: remotebrowser.New,
			state:   map[string]tftypes.Value{"id": str("1")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().GetRemoteBrowser(gomock.Any(), 1).Return(&kuma.RemoteBrowser{
					ID: 1, Name: "chrome", URL: "ws://chrome:3000",
				}, nil)
			},
		},
		{
			name:    "api key keeps the secret already in state",
			factory: apikey.New,
			state: map[string]tftypes.Value{
				"id": str("4"), "key": str("uk4_secret"),
			},
			expect: func(c *mocks.MockKumaClient) {
				// The server never returns the clear-text key again, so Read must
				// leave what state already holds alone.
				c.EXPECT().GetAPIKey(gomock.Any(), 4).Return(&kuma.APIKey{
					ID: 4, Name: "scraper", Active: kuma.Bool(true),
					Expires: ptr("2027-01-01 00:00:00"), Status: "active",
				}, nil)
			},
		},
		{
			name:    "api key with no expiry",
			factory: apikey.New,
			state:   map[string]tftypes.Value{"id": str("5")},
			expect: func(c *mocks.MockKumaClient) {
				c.EXPECT().GetAPIKey(gomock.Any(), 5).Return(&kuma.APIKey{
					ID: 5, Name: "forever", Active: kuma.Bool(false), Expires: nil, Status: "inactive",
				}, nil)
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
				t.Fatalf("unexpected diagnostics: %s", errs)
			}
			if removed {
				t.Error("a successful read must not drop the resource")
			}
		})
	}
}

// TestReadMaintenanceStrategies covers each scheduling strategy, since which
// fields the model reads back depends on which one the server reports.
func TestReadMaintenanceStrategies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		maintenance kuma.Maintenance
		monitors    []int
		pages       []int
	}{
		{
			name: "manual, where no schedule applies",
			maintenance: kuma.Maintenance{
				ID: 1, Title: "manual", Description: "d", Strategy: "manual",
				Active: kuma.BoolPtr(false), Status: "inactive",
				DateRange: []*string{nil, nil},
			},
		},
		{
			name: "single, which uses the date range",
			maintenance: kuma.Maintenance{
				ID: 2, Title: "single", Description: "d", Strategy: "single",
				Active:    kuma.BoolPtr(true),
				DateRange: []*string{ptr("2027-01-15 22:00"), ptr("2027-01-16 02:00")},
				Status:    "scheduled",
			},
			monitors: []int{10, 11},
		},
		{
			name: "recurring weekday, which uses the time range",
			maintenance: kuma.Maintenance{
				ID: 3, Title: "weekly", Description: "d", Strategy: "recurring-weekday",
				Active:    kuma.BoolPtr(true),
				DateRange: []*string{nil, nil},
				TimeRange: []kuma.TimePart{{Hours: 2, Minutes: 0}, {Hours: 4, Minutes: 30}},
				Weekdays:  []int{1, 3, 5},
				// Reads report seconds; writes take minutes.
				Duration:       ptr(9000),
				TimezoneOption: ptr("Europe/Lisbon"),
				Status:         "under-maintenance",
			},
			monitors: []int{12},
			pages:    []int{1},
		},
		{
			name: "recurring day of month, where the days arrive as JSON numbers",
			maintenance: kuma.Maintenance{
				ID: 4, Title: "monthly", Description: "d", Strategy: "recurring-day-of-month",
				DateRange: []*string{nil, nil},
				TimeRange: []kuma.TimePart{{Hours: 1, Minutes: 15}, {Hours: 2, Minutes: 45}},
				// float64, because they come through an any-typed field.
				DaysOfMonth: []any{float64(1), float64(15), 28},
				Status:      "scheduled",
			},
		},
		{
			name: "recurring interval",
			maintenance: kuma.Maintenance{
				ID: 5, Title: "every-3-days", Description: "d", Strategy: "recurring-interval",
				DateRange:   []*string{nil, nil},
				TimeRange:   []kuma.TimePart{{Hours: 0, Minutes: 0}, {Hours: 1, Minutes: 0}},
				IntervalDay: ptr(3),
				Status:      "scheduled",
			},
		},
		{
			name: "cron, which reports the expression and a duration in minutes",
			maintenance: kuma.Maintenance{
				ID: 6, Title: "cron", Description: "d", Strategy: "cron",
				DateRange:       []*string{nil, nil},
				Cron:            ptr("0 2 * * *"),
				DurationMinutes: ptr(120),
				Status:          "scheduled",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			client := mocks.NewMockKumaClient(gomock.NewController(t))
			id := tt.maintenance.ID
			client.EXPECT().GetMaintenance(gomock.Any(), id).Return(&tt.maintenance, nil)
			client.EXPECT().GetMaintenanceMonitors(gomock.Any(), id).Return(tt.monitors, nil)
			client.EXPECT().GetMaintenanceStatusPages(gomock.Any(), id).Return(tt.pages, nil)

			r := configure(t, maintenance.New, client)
			state := r.state(t, map[string]tftypes.Value{"id": str(itoa(id))})

			removed, errs := r.read(t, state)
			if errs != "" {
				t.Fatalf("unexpected diagnostics: %s", errs)
			}
			if removed {
				t.Error("a successful read must not drop the resource")
			}
		})
	}
}

// TestReadStatusPageVariants covers the page configuration together with its
// group tree, which arrives from a different transport.
func TestReadStatusPageVariants(t *testing.T) {
	t.Parallel()

	t.Run("a fully configured page with groups", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().GetStatusPage(gomock.Any(), "public").Return(&kuma.StatusPage{
			ID: 1, Slug: "public", Title: "Status",
			Description:           ptr("desc"),
			Icon:                  ptr("/icon.svg"),
			Theme:                 "dark",
			AutoRefreshInterval:   ptr(300),
			ShowTags:              kuma.BoolPtr(true),
			ShowPoweredBy:         kuma.BoolPtr(false),
			ShowCertificateExpiry: kuma.BoolPtr(true),
			ShowOnlyLastHeartbeat: kuma.BoolPtr(true),
			CustomCSS:             ptr("body{}"),
			FooterText:            ptr("footer"),
			RSSTitle:              ptr("feed"),
			AnalyticsID:           ptr("G-1"),
			AnalyticsScriptURL:    ptr("https://a/s.js"),
			AnalyticsType:         ptr("google"),
			DomainNameList:        []string{"status.example.com"},
			Published:             kuma.BoolPtr(true),
		}, nil)
		client.EXPECT().GetStatusPageGroups(gomock.Any(), "public").Return([]kuma.StatusPageGroup{
			{
				ID: 1, Name: "Core", Weight: 1,
				MonitorList: []kuma.StatusPageMonitor{
					{ID: 10, Name: "API", SendURL: kuma.BoolPtr(true), URL: ptr("https://api.example.com")},
					{ID: 11, Name: "DB", SendURL: kuma.BoolPtr(false)},
				},
			},
			{ID: 2, Name: "Empty", Weight: 2, MonitorList: nil},
		}, nil)

		r := configure(t, statuspage.New, client)
		removed, errs := r.read(t, r.state(t, map[string]tftypes.Value{"id": str("public")}))
		if errs != "" {
			t.Fatalf("unexpected diagnostics: %s", errs)
		}
		if removed {
			t.Error("a successful read must not drop the resource")
		}
	})

	t.Run("a bare page with no groups and no optional fields", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().GetStatusPage(gomock.Any(), "bare").Return(&kuma.StatusPage{
			ID: 2, Slug: "bare", Title: "Bare", Theme: "auto",
			// Empty strings and an empty domain list: the other side of every
			// branch in readInto.
			CustomCSS: ptr(""), FooterText: ptr(""), RSSTitle: ptr(""),
			DomainNameList: []string{},
		}, nil)
		client.EXPECT().GetStatusPageGroups(gomock.Any(), "bare").Return(nil, nil)

		r := configure(t, statuspage.New, client)
		if _, errs := r.read(t, r.state(t, map[string]tftypes.Value{"id": str("bare")})); errs != "" {
			t.Fatalf("unexpected diagnostics: %s", errs)
		}
	})
}

// TestReadIncidentFromHistory covers finding the incident in the page's history,
// which is how it is read: there is no getter for a single incident.
func TestReadIncidentFromHistory(t *testing.T) {
	t.Parallel()

	t.Run("the matching incident is found among others", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		client.EXPECT().GetIncidentHistory(gomock.Any(), "public").Return([]kuma.StatusPageIncident{
			{ID: 8, Title: "older", Content: "c", Style: "info", Pin: kuma.BoolPtr(false)},
			{
				ID: 9, Title: "wanted", Content: "body", Style: "danger",
				Pin: kuma.BoolPtr(true), Active: kuma.BoolPtr(true),
				CreatedDate: "2026-07-26 10:00:00",
			},
		}, nil)

		r := configure(t, statuspageincident.New, client)
		removed, errs := r.read(t, r.state(t, map[string]tftypes.Value{"id": str("public/9")}))
		if errs != "" {
			t.Fatalf("unexpected diagnostics: %s", errs)
		}
		if removed {
			t.Error("the incident is present, so it must stay in state")
		}
	})

	t.Run("an incident missing from the history is dropped", func(t *testing.T) {
		t.Parallel()

		client := mocks.NewMockKumaClient(gomock.NewController(t))
		// The page exists but the incident was deleted elsewhere.
		client.EXPECT().GetIncidentHistory(gomock.Any(), "public").Return([]kuma.StatusPageIncident{
			{ID: 8, Title: "other", Content: "c", Style: "info"},
		}, nil)

		r := configure(t, statuspageincident.New, client)
		removed, errs := r.read(t, r.state(t, map[string]tftypes.Value{"id": str("public/9")}))
		if errs != "" {
			t.Fatalf("unexpected diagnostics: %s", errs)
		}
		if !removed {
			t.Error("an incident that is gone should leave state")
		}
	})
}

// TestReadSettingsNarrowsToManagedKeys checks the settings resource reports only
// the keys it manages, so unrelated server settings never look like drift.
func TestReadSettingsNarrowsToManagedKeys(t *testing.T) {
	t.Parallel()

	client := mocks.NewMockKumaClient(gomock.NewController(t))
	client.EXPECT().GetSettings(gomock.Any()).Return(map[string]any{
		"keepDataPeriodDays": float64(200),
		"checkUpdate":        false,
		// Not managed by the configuration, so it must not leak into `settings`.
		"serverTimezone": "UTC",
	}, nil)

	r := configure(t, settings.New, client)
	state := r.state(t, map[string]tftypes.Value{
		"id":       str("settings"),
		"settings": str(`{"keepDataPeriodDays":180,"unknownToThisVersion":true}`),
	})

	if _, errs := r.read(t, state); errs != "" {
		t.Fatalf("unexpected diagnostics: %s", errs)
	}
}

// itoa avoids pulling strconv into every table above.
func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	digits := ""
	for v > 0 {
		digits = string(rune('0'+v%10)) + digits
		v /= 10
	}
	return digits
}
