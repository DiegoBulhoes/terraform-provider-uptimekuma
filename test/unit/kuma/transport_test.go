package kuma_test

import (
	"crypto/tls"
	"errors"
	"net/http"
	"testing"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
)

// The custom WebSocket used before it has dialed.
//
// This transport exists because the library's built-in one hardcodes the default
// TLS config, which makes self-signed certificates unusable — and those are the
// norm on self-hosted Uptime Kuma. Replacing it means owning the lifecycle,
// including the window before Dial where the connection is nil.
//
// Reading or writing then has to return an error. Dereferencing the nil
// connection instead would panic, and a panic in a provider kills the plugin
// process and reports a crash with no resource address.

func TestUsingTheWebSocketBeforeDialingIsAnErrorNotAPanic(t *testing.T) {
	t.Parallel()

	operations := map[string]func(*kuma.TLSWebSocketForTest) error{
		"Send": func(w *kuma.TLSWebSocketForTest) error {
			return w.Send([]byte("hello"))
		},
		"Receive": func(w *kuma.TLSWebSocketForTest) error {
			var buffer []byte
			return w.Receive(&buffer)
		},
	}

	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			socket := kuma.NewTLSWebSocketForTest(&tls.Config{MinVersion: tls.VersionTLS12})

			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("%s panicked on an undialed socket: %v", name, recovered)
				}
			}()

			err := operation(socket)
			if err == nil {
				t.Fatalf("%s on an undialed socket must return an error", name)
			}
		})
	}
}

// TestClosingAnUndialedWebSocketSucceeds is the deliberate asymmetry. Close runs
// on teardown paths that cannot know whether the dial ever happened — a failed
// connect closes the socket it never opened — so it has to be safe to call.
func TestClosingAnUndialedWebSocketSucceeds(t *testing.T) {
	t.Parallel()

	socket := kuma.NewTLSWebSocketForTest(nil)

	if err := socket.Close(); err != nil {
		t.Errorf("closing a socket that never dialed should be a no-op, got: %s", err)
	}
	// Twice, because teardown paths can overlap.
	if err := socket.Close(); err != nil {
		t.Errorf("closing twice should stay a no-op, got: %s", err)
	}
}

// TestDialingAnInvalidURLFails covers the error path of Dial itself.
func TestDialingAnInvalidURLFails(t *testing.T) {
	t.Parallel()

	socket := kuma.NewTLSWebSocketForTest(&tls.Config{MinVersion: tls.VersionTLS12})

	if err := kuma.DialForTest(socket, "not a websocket url", "http://localhost"); err == nil {
		t.Error("expected an error for a URL that is not a WebSocket endpoint")
	}
}

// TestDialingAClosedPortFails covers the other half: a well-formed URL with
// nothing listening.
func TestDialingAClosedPortFails(t *testing.T) {
	t.Parallel()

	socket := kuma.NewTLSWebSocketForTest(&tls.Config{MinVersion: tls.VersionTLS12})

	err := kuma.DialForTest(socket, "ws://127.0.0.1:1/socket.io/", "http://127.0.0.1:1")
	if err == nil {
		t.Fatal("expected a connection failure")
	}
	// And the socket must still be usable as a value afterwards, not left in a
	// state where Close panics.
	if closeErr := socket.Close(); closeErr != nil && !errors.Is(closeErr, err) {
		t.Logf("close after a failed dial: %s", closeErr)
	}
}

// TestTheHTTPClientCarriesTheTLSConfig covers the polling transport's client.
// The Engine.IO handshake happens over long polling before the upgrade to
// WebSocket, so a self-signed instance fails at the handshake unless this client
// gets the same TLS config.
func TestTheHTTPClientCarriesTheTLSConfig(t *testing.T) {
	t.Parallel()

	config := &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS12} //nolint:gosec // the point of the test

	client := kuma.HTTPClientForTest(config)
	if client == nil {
		t.Fatal("no client was built")
	}

	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected an *http.Transport, got %T", client.Transport)
	}
	if transport.TLSClientConfig == nil {
		t.Fatal("the TLS config was dropped, so a self-signed instance fails during " +
			"the polling handshake")
	}
	if !transport.TLSClientConfig.InsecureSkipVerify {
		t.Error("insecure_skip_verify did not reach the polling transport")
	}
}

// TestTheHTTPClientIsIndependentOfTheDefault guards against configuring the
// shared default transport, which would apply the provider's TLS settings to
// every other HTTP client in the process.
func TestTheHTTPClientIsIndependentOfTheDefault(t *testing.T) {
	t.Parallel()

	before := http.DefaultTransport.(*http.Transport).TLSClientConfig //nolint:errcheck // stdlib invariant

	client := kuma.HTTPClientForTest(&tls.Config{
		InsecureSkipVerify: true, //nolint:gosec // the point of the test
		MinVersion:         tls.VersionTLS12,
	})
	if client == nil {
		t.Fatal("no client was built")
	}

	after := http.DefaultTransport.(*http.Transport).TLSClientConfig //nolint:errcheck // stdlib invariant
	if before != after {
		t.Error("the default transport was modified, which would apply these TLS " +
			"settings to unrelated HTTP clients in the same process")
	}
}
