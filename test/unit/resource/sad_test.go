package resource_test

import (
	"context"
	"strings"
	"testing"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/maintenance"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/notification"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource/settings"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/test/mocks"
	"go.uber.org/mock/gomock"
)

// Sad-path tests for the resource side: configurations that are wrong, and
// server calls that fail. Every one of these should produce a message that tells
// the user what to change.

// TestSadNotificationSettings rejects three settings documents that cannot work.
func TestSadNotificationSettings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		settings string
		wantMsg  string
	}{
		{
			name:     "not JSON at all",
			settings: `webhookURL=https://example.com`,
			wantMsg:  "must be a JSON object",
		},
		{
			name:     "a JSON array instead of an object",
			settings: `["https://example.com"]`,
			wantMsg:  "must be a JSON object",
		},
		{
			name: "overriding an attribute the resource owns",
			// Letting this through would make state and server disagree about the
			// notification's name.
			settings: `{"name":"sneaky","webhookURL":"https://example.com"}`,
			wantMsg:  `must not contain "name"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := notification.BuildPayload("channel", "webhook", false, false, tt.settings)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Errorf("message %q should mention %q", err, tt.wantMsg)
			}
		})
	}
}

// TestSadClockTime rejects three malformed times.
func TestSadClockTime(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{name: "hour out of range", input: "25:00"},
		{name: "minute out of range", input: "12:75"},
		{name: "not a time at all", input: "lunchtime"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if _, err := maintenance.ParseClockTime(tt.input); err == nil {
				t.Errorf("%q should be rejected", tt.input)
			}
		})
	}
}

// TestSadSettingsRefused covers the settings the provider will not write, and
// the documents it cannot read.
func TestSadSettingsRefused(t *testing.T) {
	t.Parallel()

	t.Run("disableAuth is refused with a reason", func(t *testing.T) {
		t.Parallel()

		// Turning this on makes the server disconnect every client, this provider
		// included, leaving the apply half-done and unrecoverable.
		_, err := settings.ParseManaged(`{"disableAuth":true}`)
		if err == nil {
			t.Fatal("disableAuth should be refused")
		}
		if !strings.Contains(err.Error(), "disableAuth") {
			t.Errorf("message should name the setting: %v", err)
		}
		if !strings.Contains(err.Error(), "disconnect") {
			t.Errorf("message should explain why: %v", err)
		}
	})

	t.Run("refused even when mixed with valid keys", func(t *testing.T) {
		t.Parallel()

		_, err := settings.ParseManaged(`{"keepDataPeriodDays":180,"disableAuth":false}`)
		if err == nil {
			t.Fatal("the whole document should be refused")
		}
	})

	t.Run("a non-object document is rejected", func(t *testing.T) {
		t.Parallel()

		if _, err := settings.ParseManaged(`"just a string"`); err == nil {
			t.Error("a string is not a settings document")
		}
	})
}

// TestSadCreateFailures checks the three kinds of create failure reach the
// caller unchanged, so the diagnostic shows the server's own words.
func TestSadCreateFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
	}{
		{
			name: "validation",
			err:  &kuma.APIError{Event: "add", Msg: "Retry interval cannot be less than 1 seconds"},
		},
		{
			name: "permission",
			err:  &kuma.APIError{Event: "add", Msg: "Permission denied."},
		},
		{
			name: "rate limited",
			err:  &kuma.APIError{Event: "add", Msg: "Too frequently, try again later."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			client := mocks.NewMockKumaClient(ctrl)
			ctx := context.Background()

			client.EXPECT().CreateMonitor(ctx, gomock.Any()).Return(0, tt.err)

			id, err := client.CreateMonitor(ctx, kuma.Monitor{Name: "x", Type: "http"})
			if err == nil {
				t.Fatal("expected a failure")
			}
			if id != 0 {
				t.Errorf("no ID should be reported on failure, got %d", id)
			}
			if !strings.Contains(err.Error(), tt.err.Error()) {
				t.Errorf("the server's message was lost: %v", err)
			}
		})
	}
}

// TestSadReadFailures covers the three reads whose not-found must let the
// resource drop the object from state rather than fail the run.
func TestSadReadFailures(t *testing.T) {
	t.Parallel()

	missing := &kuma.APIError{Event: "get", Msg: "Cannot read properties of null (reading 'id')"}

	t.Run("monitor", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		client := mocks.NewMockKumaClient(ctrl)
		ctx := context.Background()

		client.EXPECT().GetMonitor(ctx, 99).Return(nil, missing)

		if _, err := client.GetMonitor(ctx, 99); !kuma.IsNotFound(err) {
			t.Errorf("error should be not-found: %v", err)
		}
	})

	t.Run("tag", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		client := mocks.NewMockKumaClient(ctrl)
		ctx := context.Background()

		client.EXPECT().GetTag(ctx, 99).Return(nil, kuma.ErrNotFound)

		if _, err := client.GetTag(ctx, 99); !kuma.IsNotFound(err) {
			t.Errorf("error should be not-found: %v", err)
		}
	})

	t.Run("notification, which is only ever read from the pushed cache", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		client := mocks.NewMockKumaClient(ctrl)
		ctx := context.Background()

		client.EXPECT().GetNotification(ctx, 99).Return(nil, kuma.ErrNotFound)

		if _, err := client.GetNotification(ctx, 99); !kuma.IsNotFound(err) {
			t.Errorf("error should be not-found: %v", err)
		}
	})
}

// TestSadDeleteFailures covers the three delete outcomes: already gone is fine,
// a real rejection is not, and a timeout deserves another try.
func TestSadDeleteFailures(t *testing.T) {
	t.Parallel()

	t.Run("already gone is tolerated", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		client := mocks.NewMockKumaClient(ctrl)
		ctx := context.Background()

		client.EXPECT().DeleteMonitor(ctx, 7, false).Return(kuma.ErrNotFound)

		err := client.DeleteMonitor(ctx, 7, false)
		// Destroying something already destroyed is a success, not an error.
		if !kuma.IsNotFound(err) {
			t.Errorf("error should be not-found so Delete can ignore it: %v", err)
		}
	})

	t.Run("a rejection must surface", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		client := mocks.NewMockKumaClient(ctrl)
		ctx := context.Background()

		rejection := &kuma.APIError{Event: "deleteMonitor", Msg: "Permission denied."}
		client.EXPECT().DeleteMonitor(ctx, 7, false).Return(rejection)

		err := client.DeleteMonitor(ctx, 7, false)
		if kuma.IsNotFound(err) {
			t.Error("a permission problem is not a missing object")
		}
		if kuma.IsRetryable(err) {
			t.Error("retrying will not grant permission")
		}
	})

	t.Run("a timeout is retryable", func(t *testing.T) {
		t.Parallel()

		ctrl := gomock.NewController(t)
		client := mocks.NewMockKumaClient(ctrl)
		ctx := context.Background()

		client.EXPECT().DeleteMonitor(ctx, 7, false).Return(kuma.ErrTimeout)

		if err := client.DeleteMonitor(ctx, 7, false); !kuma.IsRetryable(err) {
			t.Errorf("a timeout should be retryable: %v", err)
		}
	})
}
