package kuma_test

import (
	"crypto/tls"
	"errors"
	"net/http"
	"testing"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
)

// This transport exists because the library's built-in one hardcodes the default
// TLS config, which rules out self-signed certificates. Owning it means owning the
// window before Dial, where the connection is nil and a dereference would panic.

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

// The deliberate asymmetry: Close runs on teardown paths that cannot know whether
// the dial happened, so it has to be safe to call.
func TestClosingAnUndialedWebSocketSucceeds(t *testing.T) {
	t.Parallel()

	socket := kuma.NewTLSWebSocketForTest(nil)

	if err := socket.Close(); err != nil {
		t.Errorf("closing a socket that never dialed should be a no-op, got: %s", err)
	}
	// Twice: teardown paths can overlap.
	if err := socket.Close(); err != nil {
		t.Errorf("closing twice should stay a no-op, got: %s", err)
	}
}

// The error path of Dial itself.
func TestDialingAnInvalidURLFails(t *testing.T) {
	t.Parallel()

	socket := kuma.NewTLSWebSocketForTest(&tls.Config{MinVersion: tls.VersionTLS12})

	if err := kuma.DialForTest(socket, "not a websocket url", "http://localhost"); err == nil {
		t.Error("expected an error for a URL that is not a WebSocket endpoint")
	}
}

// A well-formed URL with nothing listening.
func TestDialingAClosedPortFails(t *testing.T) {
	t.Parallel()

	socket := kuma.NewTLSWebSocketForTest(&tls.Config{MinVersion: tls.VersionTLS12})

	err := kuma.DialForTest(socket, "ws://127.0.0.1:1/socket.io/", "http://127.0.0.1:1")
	if err == nil {
		t.Fatal("expected a connection failure")
	}
	// Close must still be safe afterwards.
	if closeErr := socket.Close(); closeErr != nil && !errors.Is(closeErr, err) {
		t.Logf("close after a failed dial: %s", closeErr)
	}
}

// The Engine.IO handshake runs over long polling before the WebSocket upgrade, so
// this client needs the same TLS config.
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

// Configuring the shared default transport would apply these TLS settings to
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
