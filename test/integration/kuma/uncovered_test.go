//go:build integration

package kuma_test

import (
	"context"
	"testing"
	"time"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
)

// The client exposes a few operations the resources do not use on every run:
// the "test this channel" events, tag value edits, and the list getters that go
// through the refresh path. They are still part of the API surface, so they get
// their own coverage here.

func TestClientTestEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	client, err := kuma.New(ctx, testConfig())
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	defer client.Close()

	t.Run("testNotification reaches the server", func(t *testing.T) {
		// Delivery fails — the URL is not real — but what matters is that the
		// event is accepted and the failure comes back as a server rejection
		// rather than a transport error.
		err := client.TestNotification(ctx, map[string]any{
			"name":               "probe",
			"type":               "webhook",
			"webhookURL":         "http://127.0.0.1:1/never-listening",
			"webhookContentType": "json",
		})
		if err == nil {
			t.Log("the server accepted the test notification outright")
			return
		}
		if kuma.IsRetryable(err) {
			t.Errorf("a delivery failure is the server's answer, not a transport problem: %v", err)
		}
	})

	t.Run("testDockerHost reaches the server", func(t *testing.T) {
		msg, err := client.TestDockerHost(ctx, kuma.DockerHost{
			Name:         "probe",
			DockerType:   "socket",
			DockerDaemon: "/var/run/does-not-exist.sock",
		})
		if err == nil {
			t.Logf("server reported: %s", msg)
			return
		}
		if kuma.IsRetryable(err) {
			t.Errorf("a connection failure to the daemon is a server answer: %v", err)
		}
	})
}

func TestClientListRefreshPaths(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	client, err := kuma.New(ctx, testConfig())
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	defer client.Close()

	// These three lists arrive by push but have a getter that triggers a fresh
	// one, which is the refresh path.
	t.Run("maintenances", func(t *testing.T) {
		if _, err := client.ListMaintenances(ctx); err != nil {
			t.Errorf("ListMaintenances: %v", err)
		}
	})

	t.Run("remote browsers", func(t *testing.T) {
		// Push-only: no getter exists, so this exercises the wait-for-push path.
		if _, err := client.ListRemoteBrowsers(ctx); err != nil {
			t.Errorf("ListRemoteBrowsers: %v", err)
		}
	})

	t.Run("api keys", func(t *testing.T) {
		if _, err := client.ListAPIKeys(ctx); err != nil {
			t.Errorf("ListAPIKeys: %v", err)
		}
	})
}

func TestClientEditMonitorTag(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	client, err := kuma.New(ctx, testConfig())
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	defer client.Close()

	tagID, err := client.CreateTag(ctx, kuma.Tag{Name: "acc-edit-tag", Color: "#123456"})
	if err != nil {
		t.Fatalf("CreateTag: %v", err)
	}
	t.Cleanup(func() { _ = client.DeleteTag(context.WithoutCancel(ctx), tagID) })

	url := "https://example.com"
	monitorID, err := client.CreateMonitor(ctx, kuma.Monitor{
		Name: "acc-edit-tag-monitor", Type: "http", URL: &url, Interval: 60,
	})
	if err != nil {
		t.Fatalf("CreateMonitor: %v", err)
	}
	t.Cleanup(func() { _ = client.DeleteMonitor(context.WithoutCancel(ctx), monitorID, false) })

	if err := client.AddMonitorTag(ctx, tagID, monitorID, "before"); err != nil {
		t.Fatalf("AddMonitorTag: %v", err)
	}

	// editMonitorTag exists, but the provider does not use it: the delete event
	// identifies an association by its value, so a changed value has to be a
	// detach plus an attach. This test pins down that the event itself works,
	// and documents why the resource ignores it.
	if err := client.EditMonitorTag(ctx, tagID, monitorID, "after"); err != nil {
		t.Fatalf("EditMonitorTag: %v", err)
	}

	monitor, err := client.GetMonitor(ctx, monitorID)
	if err != nil {
		t.Fatalf("GetMonitor: %v", err)
	}
	if len(monitor.Tags) != 1 {
		t.Fatalf("expected one tag, got %+v", monitor.Tags)
	}
	if monitor.Tags[0].Value == nil {
		t.Fatal("the tag lost its value")
	}
	t.Logf("value after edit: %q", *monitor.Tags[0].Value)
}

func TestClientUnpinIncident(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	client, err := kuma.New(ctx, testConfig())
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	defer client.Close()

	slug, err := client.CreateStatusPage(ctx, "Unpin Page", "acc-unpin")
	if err != nil {
		t.Fatalf("CreateStatusPage: %v", err)
	}
	t.Cleanup(func() { _ = client.DeleteStatusPage(context.WithoutCancel(ctx), slug) })

	incident, err := client.PostIncident(ctx, slug, kuma.StatusPageIncident{
		Title: "Something", Content: "Details", Style: "info",
	})
	if err != nil {
		t.Fatalf("PostIncident: %v", err)
	}
	if !incident.Pin.Value() {
		t.Fatal("posting should pin")
	}

	// unpinIncident hides the banner for the whole page without touching the
	// incident's active flag, which is what makes it different from resolving.
	if err := client.UnpinIncident(ctx, slug); err != nil {
		t.Fatalf("UnpinIncident: %v", err)
	}

	history, err := client.GetIncidentHistory(ctx, slug)
	if err != nil {
		t.Fatalf("GetIncidentHistory: %v", err)
	}
	for _, entry := range history {
		if entry.ID == incident.ID && entry.Pin.Value() {
			t.Error("the incident should no longer be pinned")
		}
	}
}

func TestClientSetupOnConfiguredInstance(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	client, err := kuma.NewUnauthenticated(ctx, kuma.Config{
		Endpoint:   testConfig().Endpoint,
		Timeout:    20 * time.Second,
		MaxRetries: 1,
	})
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	defer client.Close()

	// The instance already has an account, so needSetup is false and setup is
	// rejected. Both halves of the bootstrap path get exercised.
	need, err := client.NeedSetup(ctx)
	if err != nil {
		t.Fatalf("NeedSetup: %v", err)
	}
	if need {
		t.Skip("instance has no account; the bootstrap path is covered elsewhere")
	}

	err = client.Setup(ctx, "second-admin", "irrelevant-Passw0rd!")
	if err == nil {
		t.Error("setup should be refused once an account exists")
	} else {
		t.Logf("refused as expected: %v", err)
	}
}

// TestClientRefreshPathAfterInvalidation reaches the refresh path.
//
// It is unreachable in normal use: every pushed list arrives during login and
// stays loaded, so the getter that would refetch it never fires. Dropping the
// caches first is the only way to exercise it, and it does run for real after a
// reconnect.
func TestClientRefreshPathAfterInvalidation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	client, err := kuma.New(ctx, testConfig())
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	defer client.Close()

	// Something to find once the lists come back.
	url := "https://example.com"
	monitorID, err := client.CreateMonitor(ctx, kuma.Monitor{
		Name: "acc-refresh-target", Type: "http", URL: &url, Interval: 60,
	})
	if err != nil {
		t.Fatalf("CreateMonitor: %v", err)
	}
	t.Cleanup(func() { _ = client.DeleteMonitor(context.WithoutCancel(ctx), monitorID, false) })

	client.InvalidateCachesForTest()

	// Each of these has a getter, so they go through refreshList: emit the
	// getter, then wait for the push it triggers.
	monitors, err := client.ListMonitors(ctx)
	if err != nil {
		t.Fatalf("ListMonitors after invalidation: %v", err)
	}
	if _, found := monitors[monitorID]; !found {
		t.Errorf("monitor %d missing after a refresh of %d monitors", monitorID, len(monitors))
	}

	if _, err := client.ListMaintenances(ctx); err != nil {
		t.Errorf("ListMaintenances after invalidation: %v", err)
	}
	if _, err := client.ListAPIKeys(ctx); err != nil {
		t.Errorf("ListAPIKeys after invalidation: %v", err)
	}

	// These have no getter at all, so they take the wait-for-push branch instead.
	client.InvalidateCachesForTest()
	if _, err := client.ListNotifications(ctx); err != nil {
		t.Errorf("ListNotifications after invalidation: %v", err)
	}
	if _, err := client.ListProxies(ctx); err != nil {
		t.Errorf("ListProxies after invalidation: %v", err)
	}
	if _, err := client.ListDockerHosts(ctx); err != nil {
		t.Errorf("ListDockerHosts after invalidation: %v", err)
	}
	if _, err := client.ListRemoteBrowsers(ctx); err != nil {
		t.Errorf("ListRemoteBrowsers after invalidation: %v", err)
	}
}

// TestClientGetMissingEntities covers the not-found branch of each getter, which
// is what lets a resource drop out of state instead of failing a run.
func TestClientGetMissingEntities(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	client, err := kuma.New(ctx, testConfig())
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	defer client.Close()

	const missing = 999999

	checks := map[string]func() error{
		"monitor":        func() error { _, err := client.GetMonitor(ctx, missing); return err },
		"tag":            func() error { _, err := client.GetTag(ctx, missing); return err },
		"notification":   func() error { _, err := client.GetNotification(ctx, missing); return err },
		"proxy":          func() error { _, err := client.GetProxy(ctx, missing); return err },
		"docker host":    func() error { _, err := client.GetDockerHost(ctx, missing); return err },
		"remote browser": func() error { _, err := client.GetRemoteBrowser(ctx, missing); return err },
		"api key":        func() error { _, err := client.GetAPIKey(ctx, missing); return err },
		"maintenance":    func() error { _, err := client.GetMaintenance(ctx, missing); return err },
		"status page":    func() error { _, err := client.GetStatusPage(ctx, "no-such-slug"); return err },
	}

	for name, check := range checks {
		t.Run(name, func(t *testing.T) {
			if err := check(); !kuma.IsNotFound(err) {
				t.Errorf("a missing %s should classify as not-found, got %v", name, err)
			}
		})
	}
}

// TestClientUpdateRequiresID covers the guard that catches a caller trying to
// update something with no ID, which would otherwise create a second object.
func TestClientUpdateRequiresID(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	client, err := kuma.New(ctx, testConfig())
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	defer client.Close()

	if err := client.UpdateMonitor(ctx, kuma.Monitor{Name: "x", Type: "http"}); err == nil {
		t.Error("updating a monitor with no ID should be refused")
	}
	if err := client.UpdateMaintenance(ctx, kuma.Maintenance{Title: "x", Strategy: "manual"}); err == nil {
		t.Error("updating a maintenance with no ID should be refused")
	}
	if err := client.UpdateTag(ctx, kuma.Tag{Name: "x", Color: "#000000"}); err == nil {
		t.Error("updating a tag with no ID should be refused")
	}
}
