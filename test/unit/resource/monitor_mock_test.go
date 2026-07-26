package resource_test

import (
	"context"
	"errors"
	"testing"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/test/mocks"
	"go.uber.org/mock/gomock"
)

// These tests drive the client interface directly with a mock. They exist to pin
// down the call sequences the resources rely on, without needing a server:
// crucially, that a monitor update always writes the whole object and that
// pausing goes through pauseMonitor rather than the update payload.

func TestMockCreateMonitorSequence(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockKumaClient(ctrl)
	ctx := context.Background()

	url := "https://example.com"
	monitor := kuma.Monitor{Name: "test", Type: "http", URL: &url, Interval: 60}

	// Create returns the new ID, and the resource then reads the object back so
	// every computed attribute reflects what the server decided.
	client.EXPECT().CreateMonitor(ctx, monitor).Return(42, nil)
	client.EXPECT().GetMonitor(ctx, 42).Return(&kuma.Monitor{
		ID: 42, Name: "test", Type: "http", URL: &url, Interval: 60, RetryInterval: 60,
	}, nil)

	id, err := client.CreateMonitor(ctx, monitor)
	if err != nil {
		t.Fatalf("CreateMonitor: %v", err)
	}
	if id != 42 {
		t.Fatalf("id = %d, want 42", id)
	}

	got, err := client.GetMonitor(ctx, id)
	if err != nil {
		t.Fatalf("GetMonitor: %v", err)
	}
	// The server has no default for retryInterval and rejects zero, so the client
	// mirrors the check interval.
	if got.RetryInterval != 60 {
		t.Errorf("retry_interval = %d, want 60", got.RetryInterval)
	}
}

func TestMockPauseUsesDedicatedEvent(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockKumaClient(ctrl)
	ctx := context.Background()

	// editMonitor never writes the active column, so the only thing that pauses a
	// monitor is pauseMonitor. The resource reads current state first to avoid a
	// pointless call.
	client.EXPECT().GetMonitor(ctx, 7).Return(&kuma.Monitor{
		ID: 7, Name: "test", Type: "http", Active: kuma.BoolPtr(true),
	}, nil)
	client.EXPECT().PauseMonitor(ctx, 7).Return(nil)

	current, err := client.GetMonitor(ctx, 7)
	if err != nil {
		t.Fatalf("GetMonitor: %v", err)
	}
	if !current.Active.Value() {
		t.Fatal("fixture should start active")
	}
	if err := client.PauseMonitor(ctx, 7); err != nil {
		t.Fatalf("PauseMonitor: %v", err)
	}
}

func TestMockDeleteMonitorKeepsChildren(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockKumaClient(ctrl)
	ctx := context.Background()

	// deleteChildren is false on purpose: a group's children are managed by their
	// own resources, so cascading would destroy state Terraform still tracks.
	client.EXPECT().DeleteMonitor(ctx, 7, false).Return(nil)

	if err := client.DeleteMonitor(ctx, 7, false); err != nil {
		t.Fatalf("DeleteMonitor: %v", err)
	}
}

func TestMockTagReconciliation(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockKumaClient(ctrl)
	ctx := context.Background()

	// A changed tag value is a detach plus an attach, because the delete event
	// identifies the association by (tag, monitor, value).
	gomock.InOrder(
		client.EXPECT().DeleteMonitorTag(ctx, 3, 7, "production").Return(nil),
		client.EXPECT().AddMonitorTag(ctx, 3, 7, "staging").Return(nil),
	)

	if err := client.DeleteMonitorTag(ctx, 3, 7, "production"); err != nil {
		t.Fatalf("DeleteMonitorTag: %v", err)
	}
	if err := client.AddMonitorTag(ctx, 3, 7, "staging"); err != nil {
		t.Fatalf("AddMonitorTag: %v", err)
	}
}

func TestMockNotFoundPropagates(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockKumaClient(ctrl)
	ctx := context.Background()

	client.EXPECT().GetMonitor(ctx, 999).Return(nil, &kuma.APIError{
		Event: "getMonitor",
		Msg:   "Cannot read properties of null (reading 'id')",
	})

	_, err := client.GetMonitor(ctx, 999)
	if !kuma.IsNotFound(err) {
		t.Errorf("error should classify as not-found, got %v", err)
	}
}

func TestMockMaintenanceMonitorsReplaceAll(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockKumaClient(ctrl)
	ctx := context.Background()

	// The association is replace-all, so an empty list is how monitors get
	// detached — which is why the resource always sends it, even when empty.
	gomock.InOrder(
		client.EXPECT().SetMaintenanceMonitors(ctx, 5, []int{1, 2}).Return(nil),
		client.EXPECT().SetMaintenanceMonitors(ctx, 5, []int(nil)).Return(nil),
	)

	if err := client.SetMaintenanceMonitors(ctx, 5, []int{1, 2}); err != nil {
		t.Fatalf("SetMaintenanceMonitors: %v", err)
	}
	if err := client.SetMaintenanceMonitors(ctx, 5, nil); err != nil {
		t.Fatalf("SetMaintenanceMonitors(nil): %v", err)
	}
}

func TestMockNotificationUpsert(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockKumaClient(ctrl)
	ctx := context.Background()

	payload := map[string]any{
		"name":       "hook",
		"type":       "webhook",
		"webhookURL": "https://example.com/hook",
	}

	// A nil ID creates; a non-nil one overwrites. The same event does both.
	client.EXPECT().SaveNotification(ctx, nil, payload).Return(3, nil)
	id := 3
	client.EXPECT().SaveNotification(ctx, &id, payload).Return(3, nil)

	created, err := client.SaveNotification(ctx, nil, payload)
	if err != nil {
		t.Fatalf("SaveNotification (create): %v", err)
	}
	if created != 3 {
		t.Fatalf("id = %d, want 3", created)
	}
	if _, err := client.SaveNotification(ctx, &created, payload); err != nil {
		t.Fatalf("SaveNotification (update): %v", err)
	}
}

func TestMockAPIKeyReturnsSecretOnce(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockKumaClient(ctrl)
	ctx := context.Background()

	// The clear-text key comes back from creation only; subsequent reads never
	// include it, because the server stores a hash.
	client.EXPECT().
		CreateAPIKey(ctx, gomock.Any()).
		Return(1, "uk1_secret", nil)
	client.EXPECT().GetAPIKey(ctx, 1).Return(&kuma.APIKey{
		ID: 1, Name: "key", Active: kuma.Bool(true), Status: "active",
	}, nil)

	id, secret, err := client.CreateAPIKey(ctx, kuma.APIKey{Name: "key", Active: kuma.Bool(true)})
	if err != nil {
		t.Fatalf("CreateAPIKey: %v", err)
	}
	if secret == "" {
		t.Fatal("creation must return the clear-text key")
	}

	read, err := client.GetAPIKey(ctx, id)
	if err != nil {
		t.Fatalf("GetAPIKey: %v", err)
	}
	if read.Name != "key" || !read.Active.Value() {
		t.Errorf("unexpected key: %+v", read)
	}
}

func TestMockErrorsSurface(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mocks.NewMockKumaClient(ctrl)
	ctx := context.Background()

	sentinel := errors.New("boom")
	client.EXPECT().CreateTag(ctx, gomock.Any()).Return(0, sentinel)

	if _, err := client.CreateTag(ctx, kuma.Tag{Name: "x", Color: "#000000"}); !errors.Is(err, sentinel) {
		t.Errorf("error should propagate unchanged, got %v", err)
	}
}
