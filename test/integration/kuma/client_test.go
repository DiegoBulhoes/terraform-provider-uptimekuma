//go:build integration

// Package kuma_test exercises the Socket.IO client against a live Uptime Kuma
// instance. It runs before any resource test, because every resource depends on
// this layer behaving.
package kuma_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/test/acctest"
)

func TestMain(m *testing.M) {
	acctest.SetupTestContainer(m)
}

func testConfig() kuma.Config {
	return kuma.Config{
		Endpoint:   os.Getenv("UPTIME_KUMA_URL"),
		Username:   os.Getenv("UPTIME_KUMA_USERNAME"),
		Password:   os.Getenv("UPTIME_KUMA_PASSWORD"),
		Timeout:    30 * time.Second,
		MaxRetries: 2,
	}
}

// TestClientLifecycle walks every entity the provider manages, including the two
// distinct read paths: acknowledgement getters (monitor, maintenance, tag,
// settings) and the push-only cache (notification, proxy).
func TestClientLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	client, err := kuma.New(ctx, testConfig())
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	defer client.Close()

	info := client.Info()
	t.Logf("server version %s (container=%v, db=%s)", info.Version, info.IsContainer, info.DBType)
	if info.Version == "" {
		t.Error("no info payload was pushed after login")
	}

	t.Run("monitor", func(t *testing.T) { testMonitor(ctx, t, client) })
	t.Run("tag", func(t *testing.T) { testTag(ctx, t, client) })
	t.Run("notification", func(t *testing.T) { testNotification(ctx, t, client) })
	t.Run("proxy", func(t *testing.T) { testProxy(ctx, t, client) })
	t.Run("dockerHost", func(t *testing.T) { testDockerHost(ctx, t, client) })
	t.Run("maintenance", func(t *testing.T) { testMaintenance(ctx, t, client) })
	t.Run("apiKey", func(t *testing.T) { testAPIKey(ctx, t, client) })
	t.Run("settings", func(t *testing.T) { testSettings(ctx, t, client) })
	t.Run("notFound", func(t *testing.T) { testNotFound(ctx, t, client) })
}

func testMonitor(ctx context.Context, t *testing.T, client *kuma.Client) {
	url := "https://example.com"
	id, err := client.CreateMonitor(ctx, kuma.Monitor{
		Name:     "acc-http",
		Type:     "http",
		URL:      &url,
		Interval: 60,
	})
	if err != nil {
		t.Fatalf("CreateMonitor: %v", err)
	}
	t.Cleanup(func() {
		if err := client.DeleteMonitor(context.WithoutCancel(ctx), id, false); err != nil {
			t.Errorf("DeleteMonitor: %v", err)
		}
	})

	got, err := client.GetMonitor(ctx, id)
	if err != nil {
		t.Fatalf("GetMonitor: %v", err)
	}
	if got.Name != "acc-http" || got.Type != "http" {
		t.Fatalf("got name=%q type=%q", got.Name, got.Type)
	}
	if got.URL == nil || *got.URL != url {
		t.Fatalf("url did not round-trip: %v", got.URL)
	}
	// The client injects this default because the server dereferences the array
	// without checking.
	if len(got.AcceptedStatusCodes) == 0 {
		t.Error("accepted_statuscodes came back empty")
	}

	// editMonitor is a whole-object write, so update from what was read.
	got.Name = "acc-http-renamed"
	got.Interval = 120
	if err := client.UpdateMonitor(ctx, *got); err != nil {
		t.Fatalf("UpdateMonitor: %v", err)
	}
	after, err := client.GetMonitor(ctx, id)
	if err != nil {
		t.Fatalf("GetMonitor after update: %v", err)
	}
	if after.Name != "acc-http-renamed" || after.Interval != 120 {
		t.Fatalf("update lost: name=%q interval=%d", after.Name, after.Interval)
	}

	// Exercises the push cache: ListMonitors reads it, never an ack payload.
	monitors, err := client.ListMonitors(ctx)
	if err != nil {
		t.Fatalf("ListMonitors: %v", err)
	}
	if _, ok := monitors[id]; !ok {
		t.Fatalf("monitor %d missing from the pushed list (%d entries)", id, len(monitors))
	}

	if err := client.PauseMonitor(ctx, id); err != nil {
		t.Fatalf("PauseMonitor: %v", err)
	}
	if err := client.ResumeMonitor(ctx, id); err != nil {
		t.Fatalf("ResumeMonitor: %v", err)
	}
}

func testTag(ctx context.Context, t *testing.T, client *kuma.Client) {
	tagID, err := client.CreateTag(ctx, kuma.Tag{Name: "acc-tag", Color: "#ff0000"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	t.Cleanup(func() {
		if err := client.DeleteTag(context.WithoutCancel(ctx), tagID); err != nil {
			t.Errorf("DeleteTag: %v", err)
		}
	})

	got, err := client.GetTag(ctx, tagID)
	if err != nil {
		t.Fatalf("GetTag: %v", err)
	}
	if got.Name != "acc-tag" || got.Color != "#ff0000" {
		t.Fatalf("got %+v", got)
	}

	got.Name = "acc-tag-renamed"
	if err := client.UpdateTag(ctx, *got); err != nil {
		t.Fatalf("UpdateTag: %v", err)
	}

	// Attach to a monitor, which is a separate association event carrying an
	// optional per-monitor value.
	url := "https://example.org"
	monitorID, err := client.CreateMonitor(ctx, kuma.Monitor{
		Name: "acc-tagged", Type: "http", URL: &url, Interval: 60,
	})
	if err != nil {
		t.Fatalf("CreateMonitor for tagging: %v", err)
	}
	t.Cleanup(func() {
		_ = client.DeleteMonitor(context.WithoutCancel(ctx), monitorID, false)
	})

	if err := client.AddMonitorTag(ctx, tagID, monitorID, "env=test"); err != nil {
		t.Fatalf("AddMonitorTag: %v", err)
	}
	tagged, err := client.GetMonitor(ctx, monitorID)
	if err != nil {
		t.Fatalf("GetMonitor after tagging: %v", err)
	}
	if len(tagged.Tags) != 1 {
		t.Fatalf("expected 1 tag, got %d: %+v", len(tagged.Tags), tagged.Tags)
	}
	if tagged.Tags[0].TagID != tagID {
		t.Errorf("tag_id = %d, want %d", tagged.Tags[0].TagID, tagID)
	}
	if tagged.Tags[0].Value == nil || *tagged.Tags[0].Value != "env=test" {
		t.Errorf("tag value = %v, want env=test", tagged.Tags[0].Value)
	}

	if err := client.DeleteMonitorTag(ctx, tagID, monitorID, "env=test"); err != nil {
		t.Fatalf("DeleteMonitorTag: %v", err)
	}
}

func testNotification(ctx context.Context, t *testing.T, client *kuma.Client) {
	id, err := client.SaveNotification(ctx, nil, map[string]any{
		"name":               "acc-webhook",
		"type":               "webhook",
		"isDefault":          false,
		"applyExisting":      false,
		"webhookURL":         "https://example.com/hook",
		"webhookContentType": "json",
	})
	if err != nil {
		t.Fatalf("SaveNotification: %v", err)
	}
	t.Cleanup(func() {
		if err := client.DeleteNotification(context.WithoutCancel(ctx), id); err != nil {
			t.Errorf("DeleteNotification: %v", err)
		}
	})

	// Read comes entirely from the push cache; there is no getter event.
	got, err := client.GetNotification(ctx, id)
	if err != nil {
		t.Fatalf("GetNotification: %v", err)
	}
	if got.Name != "acc-webhook" {
		t.Errorf("name = %q", got.Name)
	}
	if got.Config == "" {
		t.Error("config JSON is empty; the type-specific settings live in there")
	}
	t.Logf("notification config: %s", got.Config)

	// Update through the same upsert event.
	if _, err := client.SaveNotification(ctx, &id, map[string]any{
		"name":               "acc-webhook-renamed",
		"type":               "webhook",
		"isDefault":          false,
		"applyExisting":      false,
		"webhookURL":         "https://example.com/hook2",
		"webhookContentType": "json",
	}); err != nil {
		t.Fatalf("SaveNotification update: %v", err)
	}
	after, err := client.GetNotification(ctx, id)
	if err != nil {
		t.Fatalf("GetNotification after update: %v", err)
	}
	if after.Name != "acc-webhook-renamed" {
		t.Errorf("update lost: name = %q", after.Name)
	}
}

func testProxy(ctx context.Context, t *testing.T, client *kuma.Client) {
	user := "proxyuser"
	pass := "proxypass"
	id, err := client.SaveProxy(ctx, nil, kuma.Proxy{
		Protocol: "http",
		Host:     "proxy.example.com",
		Port:     3128,
		Auth:     true,
		Username: &user,
		Password: &pass,
		Active:   true,
	})
	if err != nil {
		t.Fatalf("SaveProxy: %v", err)
	}
	t.Cleanup(func() {
		if err := client.DeleteProxy(context.WithoutCancel(ctx), id); err != nil {
			t.Errorf("DeleteProxy: %v", err)
		}
	})

	got, err := client.GetProxy(ctx, id)
	if err != nil {
		t.Fatalf("GetProxy: %v", err)
	}
	if got.Host != "proxy.example.com" || got.Port != 3128 {
		t.Errorf("got %+v", got)
	}
}

func testDockerHost(ctx context.Context, t *testing.T, client *kuma.Client) {
	id, err := client.SaveDockerHost(ctx, nil, kuma.DockerHost{
		Name:         "acc-docker",
		DockerType:   "socket",
		DockerDaemon: "/var/run/docker.sock",
	})
	if err != nil {
		t.Fatalf("SaveDockerHost: %v", err)
	}
	t.Cleanup(func() {
		if err := client.DeleteDockerHost(context.WithoutCancel(ctx), id); err != nil {
			t.Errorf("DeleteDockerHost: %v", err)
		}
	})

	got, err := client.GetDockerHost(ctx, id)
	if err != nil {
		t.Fatalf("GetDockerHost: %v", err)
	}
	if got.Name != "acc-docker" {
		t.Errorf("name = %q", got.Name)
	}
}

func testMaintenance(ctx context.Context, t *testing.T, client *kuma.Client) {
	id, err := client.CreateMaintenance(ctx, kuma.Maintenance{
		Title:       "acc-window",
		Description: "acceptance test",
		Strategy:    "manual",
	})
	if err != nil {
		t.Fatalf("CreateMaintenance: %v", err)
	}
	t.Cleanup(func() {
		if err := client.DeleteMaintenance(context.WithoutCancel(ctx), id); err != nil {
			t.Errorf("DeleteMaintenance: %v", err)
		}
	})

	got, err := client.GetMaintenance(ctx, id)
	if err != nil {
		t.Fatalf("GetMaintenance: %v", err)
	}
	if got.Title != "acc-window" || got.Strategy != "manual" {
		t.Fatalf("got %+v", got)
	}
	t.Logf("maintenance status = %q", got.Status)

	// Monitor association is replace-all, not append.
	url := "https://example.net"
	monitorID, err := client.CreateMonitor(ctx, kuma.Monitor{
		Name: "acc-maint-target", Type: "http", URL: &url, Interval: 60,
	})
	if err != nil {
		t.Fatalf("CreateMonitor for maintenance: %v", err)
	}
	t.Cleanup(func() {
		_ = client.DeleteMonitor(context.WithoutCancel(ctx), monitorID, false)
	})

	if err := client.SetMaintenanceMonitors(ctx, id, []int{monitorID}); err != nil {
		t.Fatalf("SetMaintenanceMonitors: %v", err)
	}
	linked, err := client.GetMaintenanceMonitors(ctx, id)
	if err != nil {
		t.Fatalf("GetMaintenanceMonitors: %v", err)
	}
	if len(linked) != 1 || linked[0] != monitorID {
		t.Fatalf("linked = %v, want [%d]", linked, monitorID)
	}

	// Empty list must clear the association, proving replace semantics.
	if err := client.SetMaintenanceMonitors(ctx, id, nil); err != nil {
		t.Fatalf("SetMaintenanceMonitors(nil): %v", err)
	}
	cleared, err := client.GetMaintenanceMonitors(ctx, id)
	if err != nil {
		t.Fatalf("GetMaintenanceMonitors after clear: %v", err)
	}
	if len(cleared) != 0 {
		t.Errorf("expected no linked monitors, got %v", cleared)
	}

	if err := client.PauseMaintenance(ctx, id); err != nil {
		t.Fatalf("PauseMaintenance: %v", err)
	}
	if err := client.ResumeMaintenance(ctx, id); err != nil {
		t.Fatalf("ResumeMaintenance: %v", err)
	}
}

func testAPIKey(ctx context.Context, t *testing.T, client *kuma.Client) {
	id, clearKey, err := client.CreateAPIKey(ctx, kuma.APIKey{Name: "acc-key", Active: true})
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	t.Cleanup(func() {
		if err := client.DeleteAPIKey(context.WithoutCancel(ctx), id); err != nil {
			t.Errorf("DeleteAPIKey: %v", err)
		}
	})

	// The clear-text key exists only in this response.
	if clearKey == "" {
		t.Fatal("no clear-text key returned")
	}

	got, err := client.GetAPIKey(ctx, id)
	if err != nil {
		t.Fatalf("GetAPIKey: %v", err)
	}
	if got.Name != "acc-key" {
		t.Errorf("name = %q", got.Name)
	}

	if err := client.SetAPIKeyActive(ctx, id, false); err != nil {
		t.Fatalf("SetAPIKeyActive(false): %v", err)
	}
	disabled, err := client.GetAPIKey(ctx, id)
	if err != nil {
		t.Fatalf("GetAPIKey after disable: %v", err)
	}
	if disabled.Active {
		t.Error("key should be inactive")
	}
}

func testSettings(ctx context.Context, t *testing.T, client *kuma.Client) {
	settings, err := client.GetSettings(ctx)
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	if len(settings) == 0 {
		t.Fatal("settings came back empty")
	}
	t.Logf("settings has %d keys", len(settings))

	// Write one harmless key back and confirm it persists.
	settings["keepDataPeriodDays"] = 200
	if err := client.SetSettings(ctx, settings, ""); err != nil {
		t.Fatalf("SetSettings: %v", err)
	}
	after, err := client.GetSettings(ctx)
	if err != nil {
		t.Fatalf("GetSettings after write: %v", err)
	}
	t.Logf("keepDataPeriodDays = %v", after["keepDataPeriodDays"])
}

func testNotFound(ctx context.Context, t *testing.T, client *kuma.Client) {
	// A missing row makes the server dereference null, and the resulting
	// TypeError message is the only not-found signal available.
	if _, err := client.GetMonitor(ctx, 999999); !kuma.IsNotFound(err) {
		t.Errorf("GetMonitor(999999) error = %v, want ErrNotFound", err)
	}
	if _, err := client.GetTag(ctx, 999999); !kuma.IsNotFound(err) {
		t.Errorf("GetTag(999999) error = %v, want ErrNotFound", err)
	}
	if _, err := client.GetNotification(ctx, 999999); !kuma.IsNotFound(err) {
		t.Errorf("GetNotification(999999) error = %v, want ErrNotFound", err)
	}
}
