package kuma_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
)

// Sad-path tests: the server said no, or the connection did. What matters is
// that each failure lands in the right bucket, because the buckets decide
// whether the provider retries, removes the object from state, or gives up.

// TestSadServerRejections covers three rejections the server produces in normal
// operation, none of which should be retried.
func TestSadServerRejections(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		event string
		msg   string
	}{
		{
			name:  "validation error from Monitor.validate",
			event: "add",
			msg:   "Retry interval cannot be less than 1 seconds",
		},
		{
			name:  "another user's monitor",
			event: "editMonitor",
			msg:   "Permission denied.",
		},
		{
			name:  "column missing on an older server",
			event: "add",
			msg:   "SQLITE_ERROR: table monitor has no column named bearer_token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := error(&kuma.APIError{Event: tt.event, Msg: tt.msg})

			// Replaying the same payload would fail the same way.
			if kuma.IsRetryable(err) {
				t.Error("a rejected payload must not be retried")
			}
			if kuma.IsNotFound(err) {
				t.Error("a rejection is not a missing object")
			}
			// The message has to name the event: the server's own text is often
			// just an i18n key.
			if got := err.Error(); !strings.Contains(got, tt.event) || !strings.Contains(got, tt.msg) {
				t.Errorf("error text lost information: %q", got)
			}
		})
	}
}

// TestSadMissingObjects covers the three ways the server reports something that
// is no longer there. All must map onto ErrNotFound so Read can drop the object
// from state instead of failing the run.
func TestSadMissingObjects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		event string
		msg   string
	}{
		{
			name:  "getMonitor dereferences a null bean",
			event: "getMonitor",
			msg:   "Cannot read properties of null (reading 'id')",
		},
		{
			name:  "editTag answers with an i18n key",
			event: "editTag",
			msg:   "tagNotFound",
		},
		{
			name:  "Proxy.save says it plainly",
			event: "addProxy",
			msg:   "proxy not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := error(&kuma.APIError{Event: tt.event, Msg: tt.msg})

			if !kuma.IsNotFound(err) {
				t.Errorf("%q should mean not-found", tt.msg)
			}
			// Retrying will not bring it back.
			if kuma.IsRetryable(err) {
				t.Error("a missing object must not be retried")
			}
			// Classification has to survive the wrapping the client adds.
			wrapped := fmt.Errorf("reading monitor 7: %w", fmt.Errorf("%q: %w", tt.event, err))
			if !kuma.IsNotFound(wrapped) {
				t.Error("wrapping must not hide the classification")
			}
		})
	}
}

// TestSadTransportFailures covers three failures that are worth another attempt,
// since none of them says anything about the request being wrong.
func TestSadTransportFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
	}{
		{name: "no acknowledgement arrived", err: kuma.ErrTimeout},
		{name: "session is gone", err: kuma.ErrNotConnected},
		{name: "socket died mid-call", err: errors.New("use of closed network connection")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if !kuma.IsRetryable(tt.err) {
				t.Errorf("%v should be retryable", tt.err)
			}
			if kuma.IsNotFound(tt.err) {
				t.Error("a transport failure says nothing about the object existing")
			}
			// Still retryable once the client has wrapped it with context.
			wrapped := fmt.Errorf("emitting %q: %w", "add", tt.err)
			if !kuma.IsRetryable(wrapped) {
				t.Error("wrapping must not hide retryability")
			}
		})
	}
}

// TestSadRateLimit covers the login limiter, which is reachable in ordinary use:
// 20 attempts a minute for the whole server, and every provider instance spends
// one.
func TestSadRateLimit(t *testing.T) {
	t.Parallel()

	t.Run("the limiter message is recognized", func(t *testing.T) {
		t.Parallel()

		err := error(&kuma.APIError{Event: "login", Msg: "Too frequently, try again later."})
		if !kuma.IsRateLimited(err) {
			t.Error("should be recognized as rate limiting")
		}
	})

	t.Run("rate limiting is retryable, unlike other rejections", func(t *testing.T) {
		t.Parallel()

		err := error(&kuma.APIError{Event: "login", Msg: "Too frequently, try again later."})
		if !kuma.IsRetryable(err) {
			t.Error("waiting is the only fix, so it must be retried")
		}
	})

	t.Run("bad credentials are not rate limiting", func(t *testing.T) {
		t.Parallel()

		err := error(&kuma.APIError{Event: "login", Msg: "authIncorrectCreds"})
		if kuma.IsRateLimited(err) {
			t.Error("wrong credentials is a different failure")
		}
		if kuma.IsRetryable(err) {
			t.Error("retrying will not fix a wrong password")
		}
	})
}
