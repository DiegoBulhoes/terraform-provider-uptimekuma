//go:build integration

package kuma_test

import (
	"context"
	"testing"
	"time"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
)

// TestStatusPageLifecycle exercises the status page API, which is the only part
// of the provider that needs both Socket.IO and HTTP: the group tree is not
// exposed by any event.
func TestStatusPageLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	client, err := kuma.New(ctx, testConfig())
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	defer client.Close()

	slug, err := client.CreateStatusPage(ctx, "Acceptance Page", "acc-page")
	if err != nil {
		t.Fatalf("CreateStatusPage: %v", err)
	}
	t.Cleanup(func() {
		if err := client.DeleteStatusPage(context.WithoutCancel(ctx), slug); err != nil {
			t.Errorf("DeleteStatusPage: %v", err)
		}
	})
	t.Logf("created status page %q", slug)

	// A monitor to put on the page.
	url := "https://example.com"
	monitorID, err := client.CreateMonitor(ctx, kuma.Monitor{
		Name: "acc-page-monitor", Type: "http", URL: &url, Interval: 60,
	})
	if err != nil {
		t.Fatalf("CreateMonitor: %v", err)
	}
	t.Cleanup(func() {
		_ = client.DeleteMonitor(context.WithoutCancel(ctx), monitorID, false)
	})

	// ── Save configuration and groups ─────────────────────────────────
	description := "Managed by the acceptance test"
	footer := "footer text"
	groups, err := client.SaveStatusPage(ctx, slug, kuma.StatusPage{
		Slug:        slug,
		Title:       "Acceptance Page",
		Description: &description,
		Theme:       "dark",
		FooterText:  &footer,
		ShowTags:    kuma.BoolPtr(true),
	}, "/icon.svg", []kuma.StatusPageGroup{
		{
			Name: "Core",
			MonitorList: []kuma.StatusPageMonitor{
				{ID: monitorID, SendURL: kuma.BoolPtr(true)},
			},
		},
		{
			Name:        "Empty on purpose",
			MonitorList: []kuma.StatusPageMonitor{},
		},
	})
	if err != nil {
		t.Fatalf("SaveStatusPage: %v", err)
	}
	// The acknowledgement returns the groups with server-assigned IDs.
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups back, got %d: %+v", len(groups), groups)
	}
	if groups[0].ID == 0 {
		t.Error("the saved group should come back with an ID")
	}
	t.Logf("groups: %+v", groups)

	// ── Read the configuration over Socket.IO ─────────────────────────
	config, err := client.GetStatusPage(ctx, slug)
	if err != nil {
		t.Fatalf("GetStatusPage: %v", err)
	}
	if config.Title != "Acceptance Page" || config.Theme != "dark" {
		t.Errorf("config did not round-trip: %+v", config)
	}
	if config.Description == nil || *config.Description != description {
		t.Errorf("description = %v", config.Description)
	}
	if !config.ShowTags.Value() {
		t.Error("show_tags should be true")
	}

	// ── Read the group tree over HTTP ─────────────────────────────────
	readGroups, err := client.GetStatusPageGroups(ctx, slug)
	if err != nil {
		t.Fatalf("GetStatusPageGroups: %v", err)
	}
	if len(readGroups) != 2 {
		t.Fatalf("expected 2 groups over HTTP, got %d: %+v", len(readGroups), readGroups)
	}
	// Order matters: the server derives weight from position.
	if readGroups[0].Name != "Core" || readGroups[1].Name != "Empty on purpose" {
		t.Errorf("group order was not preserved: %q, %q", readGroups[0].Name, readGroups[1].Name)
	}
	if len(readGroups[0].MonitorList) != 1 || readGroups[0].MonitorList[0].ID != monitorID {
		t.Errorf("monitor list = %+v", readGroups[0].MonitorList)
	}

	// ── Removing a group deletes it ───────────────────────────────────
	if _, err := client.SaveStatusPage(ctx, slug, kuma.StatusPage{
		Slug: slug, Title: "Acceptance Page", Theme: "dark",
	}, "/icon.svg", []kuma.StatusPageGroup{
		{Name: "Core", MonitorList: []kuma.StatusPageMonitor{{ID: monitorID}}},
	}); err != nil {
		t.Fatalf("SaveStatusPage with fewer groups: %v", err)
	}
	afterRemoval, err := client.GetStatusPageGroups(ctx, slug)
	if err != nil {
		t.Fatalf("GetStatusPageGroups after removal: %v", err)
	}
	if len(afterRemoval) != 1 {
		t.Errorf("a group missing from the list should be deleted, got %d groups", len(afterRemoval))
	}

	// ── The page appears in the pushed list ───────────────────────────
	// refresh=true: the list is pushed only during afterLogin, so a page created
	// in this session is invisible without reconnecting.
	pages, err := client.ListStatusPages(ctx, true)
	if err != nil {
		t.Fatalf("ListStatusPages: %v", err)
	}
	found := false
	for _, page := range pages {
		if page.Slug == slug {
			found = true
		}
	}
	if !found {
		t.Errorf("status page %q missing from the pushed list of %d", slug, len(pages))
	}

	t.Run("incident", func(t *testing.T) { testIncident(ctx, t, client, slug) })
	t.Run("maintenanceLink", func(t *testing.T) { testMaintenanceStatusPage(ctx, t, client, pages, slug) })
	t.Run("notFound", func(t *testing.T) {
		if _, err := client.GetStatusPage(ctx, "no-such-page"); !kuma.IsNotFound(err) {
			t.Errorf("unknown slug should be not-found, got %v", err)
		}
		if _, err := client.GetStatusPageGroups(ctx, "no-such-page"); !kuma.IsNotFound(err) {
			t.Errorf("unknown slug over HTTP should be not-found, got %v", err)
		}
	})
}

func testIncident(ctx context.Context, t *testing.T, client *kuma.Client, slug string) {
	incident, err := client.PostIncident(ctx, slug, kuma.StatusPageIncident{
		Title:   "Degraded performance",
		Content: "We are looking into it.",
		Style:   "warning",
	})
	if err != nil {
		t.Fatalf("PostIncident: %v", err)
	}
	if incident.ID == 0 {
		t.Fatal("the incident should come back with an ID")
	}
	if !incident.Pin.Value() {
		t.Error("posting an incident pins it")
	}
	t.Logf("incident %d: %s", incident.ID, incident.Title)

	updated, err := client.EditIncident(ctx, slug, incident.ID, kuma.StatusPageIncident{
		Title:   "Still degraded",
		Content: "Working on a fix.",
		Style:   "danger",
	})
	if err != nil {
		t.Fatalf("EditIncident: %v", err)
	}
	if updated != nil && updated.Title != "Still degraded" {
		t.Errorf("edit did not stick: %+v", updated)
	}

	if err := client.ResolveIncident(ctx, slug, incident.ID); err != nil {
		t.Fatalf("ResolveIncident: %v", err)
	}

	history, err := client.GetIncidentHistory(ctx, slug)
	if err != nil {
		t.Fatalf("GetIncidentHistory: %v", err)
	}
	t.Logf("history has %d incidents", len(history))

	if err := client.DeleteIncident(ctx, slug, incident.ID); err != nil {
		t.Fatalf("DeleteIncident: %v", err)
	}
}

func testMaintenanceStatusPage(ctx context.Context, t *testing.T, client *kuma.Client, pages map[int]kuma.StatusPage, slug string) {
	var pageID int
	for id, page := range pages {
		if page.Slug == slug {
			pageID = id
		}
	}
	if pageID == 0 {
		t.Skip("status page ID not known")
	}

	maintenanceID, err := client.CreateMaintenance(ctx, kuma.Maintenance{
		Title: "acc-page-window", Description: "d", Strategy: "manual",
	})
	if err != nil {
		t.Fatalf("CreateMaintenance: %v", err)
	}
	t.Cleanup(func() {
		_ = client.DeleteMaintenance(context.WithoutCancel(ctx), maintenanceID)
	})

	if err := client.SetMaintenanceStatusPages(ctx, maintenanceID, []int{pageID}); err != nil {
		t.Fatalf("SetMaintenanceStatusPages: %v", err)
	}
	linked, err := client.GetMaintenanceStatusPages(ctx, maintenanceID)
	if err != nil {
		t.Fatalf("GetMaintenanceStatusPages: %v", err)
	}
	if len(linked) != 1 || linked[0] != pageID {
		t.Errorf("linked pages = %v, want [%d]", linked, pageID)
	}

	// Replace-all: an empty list clears the association.
	if err := client.SetMaintenanceStatusPages(ctx, maintenanceID, nil); err != nil {
		t.Fatalf("SetMaintenanceStatusPages(nil): %v", err)
	}
	cleared, err := client.GetMaintenanceStatusPages(ctx, maintenanceID)
	if err != nil {
		t.Fatalf("GetMaintenanceStatusPages after clear: %v", err)
	}
	if len(cleared) != 0 {
		t.Errorf("expected no linked pages, got %v", cleared)
	}
}
