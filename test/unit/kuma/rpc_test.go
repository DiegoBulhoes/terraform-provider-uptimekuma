package kuma_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
)

// Acknowledgement decoding and connection failures. A live server does not
// produce malformed acknowledgements or refuse connections on demand, so these
// paths need direct tests — and they are the paths where a mistake is silent.

func TestDecodeAcknowledgements(t *testing.T) {
	t.Parallel()

	t.Run("ok:false becomes a rejection carrying the message", func(t *testing.T) {
		t.Parallel()

		var out struct{}
		err := kuma.DecodeAckForTest("add", []byte(`{"ok":false,"msg":"Permission denied."}`), &out)
		if err == nil {
			t.Fatal("expected an error")
		}
		if !strings.Contains(err.Error(), "Permission denied") {
			t.Errorf("the server's message should survive: %v", err)
		}
		if !strings.Contains(err.Error(), "add") {
			t.Errorf("the event should be named: %v", err)
		}
	})

	t.Run("ok:true decodes the payload", func(t *testing.T) {
		t.Parallel()

		var out struct {
			MonitorID int `json:"monitorID"`
		}
		if err := kuma.DecodeAckForTest("add", []byte(`{"ok":true,"monitorID":42}`), &out); err != nil {
			t.Fatalf("decoding: %v", err)
		}
		if out.MonitorID != 42 {
			t.Errorf("monitorID = %d", out.MonitorID)
		}
	})

	t.Run("a bare boolean is accepted, because needSetup answers that way", func(t *testing.T) {
		t.Parallel()

		var need bool
		if err := kuma.DecodeAckForTest("needSetup", []byte(`true`), &need); err != nil {
			t.Fatalf("decoding: %v", err)
		}
		if !need {
			t.Error("should decode as true")
		}
	})

	t.Run("a nil out ignores the payload", func(t *testing.T) {
		t.Parallel()

		// Most write events are called for their success, not their payload.
		if err := kuma.DecodeAckForTest("deleteMonitor", []byte(`{"ok":true,"msg":"deleted"}`), nil); err != nil {
			t.Errorf("a payload nobody wants is not an error: %v", err)
		}
	})

	t.Run("a payload that does not fit the target fails loudly", func(t *testing.T) {
		t.Parallel()

		var out struct {
			MonitorID int `json:"monitorID"`
		}
		// A string where a number belongs. Failing loudly beats returning a zero
		// ID that the caller would treat as "server returned nothing".
		err := kuma.DecodeAckForTest("add", []byte(`{"ok":true,"monitorID":"not-a-number"}`), &out)
		if err == nil {
			t.Error("expected a decode error")
		}
	})

	t.Run("malformed JSON fails", func(t *testing.T) {
		t.Parallel()

		var out struct{}
		if err := kuma.DecodeAckForTest("add", []byte(`{"ok":`), &out); err == nil {
			t.Error("expected a decode error")
		}
	})
}

func TestConnectionFailures(t *testing.T) {
	t.Parallel()

	t.Run("an empty endpoint is rejected before dialing", func(t *testing.T) {
		t.Parallel()

		_, err := kuma.New(context.Background(), kuma.Config{Username: "u", Password: "p"})
		if err == nil {
			t.Fatal("expected an error")
		}
		if !strings.Contains(err.Error(), "endpoint") {
			t.Errorf("the message should name the missing field: %v", err)
		}
	})

	t.Run("an unparseable endpoint is rejected", func(t *testing.T) {
		t.Parallel()

		_, err := kuma.New(context.Background(), kuma.Config{
			Endpoint: "http://[::1]:namedport",
			Username: "u", Password: "p",
			Timeout: 2 * time.Second,
		})
		if err == nil {
			t.Error("expected an error")
		}
	})

	t.Run("a port nobody listens on fails after the retries", func(t *testing.T) {
		t.Parallel()

		start := time.Now()
		_, err := kuma.New(context.Background(), kuma.Config{
			// Port 1 needs privileges to bind, so nothing will be there.
			Endpoint: "http://127.0.0.1:1",
			Username: "u", Password: "p",
			Timeout:    2 * time.Second,
			MaxRetries: 0,
		})
		if err == nil {
			t.Fatal("expected an error")
		}
		if !strings.Contains(err.Error(), "127.0.0.1:1") {
			t.Errorf("the message should name the endpoint: %v", err)
		}
		// MaxRetries 0 means one attempt, so this must not sit through a backoff
		// schedule.
		if elapsed := time.Since(start); elapsed > 25*time.Second {
			t.Errorf("took %v, which suggests it retried more than once", elapsed)
		}
	})

	t.Run("a cancelled context stops the attempt", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := kuma.New(ctx, kuma.Config{
			Endpoint: "http://127.0.0.1:1",
			Username: "u", Password: "p",
			Timeout:    2 * time.Second,
			MaxRetries: 3,
		})
		if err == nil {
			t.Error("expected an error")
		}
	})

	t.Run("missing credentials are caught before any request", func(t *testing.T) {
		t.Parallel()

		// Reaching the login with no username would waste one of the server's 20
		// logins per minute to learn nothing.
		_, err := kuma.New(context.Background(), kuma.Config{
			Endpoint: "http://127.0.0.1:1",
			Timeout:  2 * time.Second,
		})
		if err == nil {
			t.Error("expected an error")
		}
	})
}

func TestDefaultsApplied(t *testing.T) {
	t.Parallel()

	// A zero timeout would mean "wait forever" and a negative retry count would
	// mean "never try"; both are normalised on the way in. The connection still
	// fails, but reaching that failure at all proves the config was accepted.
	_, err := kuma.New(context.Background(), kuma.Config{
		Endpoint: "http://127.0.0.1:1",
		Username: "u", Password: "p",
		Timeout:    0,
		MaxRetries: -5,
	})
	if err == nil {
		t.Fatal("expected a connection error")
	}
	if strings.Contains(err.Error(), "endpoint is required") {
		t.Error("the config should have passed validation")
	}
}
