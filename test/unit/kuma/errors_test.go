package kuma_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
)

// TestAPIErrorNotFound covers the message matching that stands in for a
// not-found status.
//
// Uptime Kuma has no error codes: a getter for a missing row dereferences a null
// bean and the server reports the resulting JavaScript TypeError. Recognizing
// those messages is what lets Read drop a deleted object from state instead of
// failing the whole run.
func TestAPIErrorNotFound(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		msg      string
		notFound bool
	}{
		{name: "null bean dereference", msg: "Cannot read properties of null (reading 'id')", notFound: true},
		{name: "undefined dereference", msg: "Cannot read properties of undefined (reading 'toJSON')", notFound: true},
		{name: "spaced not found", msg: "Monitor not found", notFound: true},
		{name: "i18n key", msg: "tagNotFound", notFound: true},
		{name: "proxy not found", msg: "proxy not found", notFound: true},
		{name: "permission denied is not not-found", msg: "Permission denied.", notFound: false},
		{name: "validation error is not not-found", msg: "Interval cannot be less than 20 seconds", notFound: false},
		{name: "rate limit is not not-found", msg: "Too frequently, try again later.", notFound: false},
		{name: "empty", msg: "", notFound: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := error(&kuma.APIError{Event: "getMonitor", Msg: tt.msg})
			if got := kuma.IsNotFound(err); got != tt.notFound {
				t.Errorf("IsNotFound(%q) = %v, want %v", tt.msg, got, tt.notFound)
			}
		})
	}
}

// TestAPIErrorNotFoundThroughWrapping makes sure the classification survives the
// fmt.Errorf wrapping the client layers on.
func TestAPIErrorNotFoundThroughWrapping(t *testing.T) {
	t.Parallel()

	base := &kuma.APIError{Event: "getMonitor", Msg: "Cannot read properties of null (reading 'id')"}
	wrapped := fmt.Errorf("reading monitor 7: %w", fmt.Errorf("%q: %w", "getMonitor", base))

	if !kuma.IsNotFound(wrapped) {
		t.Error("wrapping must not hide a not-found error")
	}
}

// TestRateLimitDetection covers the login limiter, which is reachable in normal
// use: it allows 20 attempts a minute for the whole server and every provider
// instance spends one.
func TestRateLimitDetection(t *testing.T) {
	t.Parallel()

	limited := &kuma.APIError{Event: "login", Msg: "Too frequently, try again later."}
	if !kuma.IsRateLimited(limited) {
		t.Error("the limiter message should be recognized")
	}
	if !kuma.IsRetryable(limited) {
		t.Error("a rate-limited call must be retryable; only waiting fixes it")
	}

	other := &kuma.APIError{Event: "login", Msg: "authIncorrectCreds"}
	if kuma.IsRateLimited(other) {
		t.Error("wrong credentials is not a rate limit")
	}
}

// TestIsRetryable pins down which failures are worth repeating.
func TestIsRetryable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "timeout", err: kuma.ErrTimeout, want: true},
		{name: "not connected", err: kuma.ErrNotConnected, want: true},
		{name: "wrapped timeout", err: fmt.Errorf("emitting: %w", kuma.ErrTimeout), want: true},
		{name: "not found", err: kuma.ErrNotFound, want: false},
		{
			name: "server rejection",
			err:  &kuma.APIError{Event: "add", Msg: "Interval cannot be less than 20 seconds"},
			want: false,
		},
		{
			name: "not-found rejection",
			err:  &kuma.APIError{Event: "getMonitor", Msg: "Cannot read properties of null"},
			want: false,
		},
		{name: "transport failure", err: errors.New("connection reset by peer"), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := kuma.IsRetryable(tt.err); got != tt.want {
				t.Errorf("IsRetryable(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// TestAPIErrorMessage checks the error text names the event, since the server's
// own messages are often just an i18n key.
func TestAPIErrorMessage(t *testing.T) {
	t.Parallel()

	withMsg := (&kuma.APIError{Event: "editMonitor", Msg: "Permission denied."}).Error()
	if want := `uptime kuma rejected "editMonitor": Permission denied.`; withMsg != want {
		t.Errorf("got %q, want %q", withMsg, want)
	}

	withoutMsg := (&kuma.APIError{Event: "editMonitor"}).Error()
	if want := `uptime kuma rejected "editMonitor"`; withoutMsg != want {
		t.Errorf("got %q, want %q", withoutMsg, want)
	}
}
