package kuma_test

import (
	"context"
	"strings"
	"testing"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
)

// context.WithCancel(nil) panics, so a client with no base context used to kill
// the plugin process on its first dial. A panic leaves the operation half-applied
// with nothing in state.

// The direct regression: this used to panic inside context.WithCancel.
func TestDialingWithoutABaseContextFailsInsteadOfPanicking(t *testing.T) {
	t.Parallel()

	// Port 1 is closed; the point is *how* the dial fails.
	client := kuma.NewWithoutBaseContextForTest("http://127.0.0.1:1")

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("dialing without a base context panicked instead of returning an "+
				"error, which would crash the provider process: %v", recovered)
		}
	}()

	_, err := client.ListMonitors(context.Background())
	if err == nil {
		t.Fatal("a client that cannot reach a server must return an error")
	}
}

// Every entry point reaches the dial through session(); the panic was found
// through DeleteStatusPage, not ListMonitors.
func TestEveryClientMethodSurvivesAMissingBaseContext(t *testing.T) {
	t.Parallel()

	cases := map[string]func(*kuma.Client, context.Context) error{
		"ListMonitors": func(c *kuma.Client, ctx context.Context) error {
			_, err := c.ListMonitors(ctx)
			return err
		},
		"DeleteStatusPage": func(c *kuma.Client, ctx context.Context) error {
			return c.DeleteStatusPage(ctx, "slug")
		},
		"GetSettings": func(c *kuma.Client, ctx context.Context) error {
			_, err := c.GetSettings(ctx)
			return err
		},
		"CreateMonitor": func(c *kuma.Client, ctx context.Context) error {
			_, err := c.CreateMonitor(ctx, kuma.Monitor{Name: "n", Type: "http"})
			return err
		},
		"ListNotifications": func(c *kuma.Client, ctx context.Context) error {
			_, err := c.ListNotifications(ctx)
			return err
		},
		"NeedSetup": func(c *kuma.Client, ctx context.Context) error {
			_, err := c.NeedSetup(ctx)
			return err
		},
	}

	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			client := kuma.NewWithoutBaseContextForTest("http://127.0.0.1:1")

			defer func() {
				if recovered := recover(); recovered != nil {
					t.Fatalf("%s panicked with no base context: %v", name, recovered)
				}
			}()

			if err := call(client, context.Background()); err == nil {
				t.Errorf("%s should have failed against a closed port", name)
			}
		})
	}
}

// The root cause: a constructor that forgets the field fails here rather than
// panicking in production.
func TestEveryClientHasABaseContext(t *testing.T) {
	t.Parallel()

	configs := map[string]kuma.Config{
		"minimal":          {Endpoint: "http://127.0.0.1:1"},
		"with credentials": {Endpoint: "http://127.0.0.1:1", Username: "admin", Password: "p"},
		"zero timeout":     {Endpoint: "http://127.0.0.1:1", Timeout: 0},
		"negative retries": {Endpoint: "http://127.0.0.1:1", MaxRetries: -5},
		"insecure":         {Endpoint: "https://127.0.0.1:1", InsecureSkipVerify: true},
	}

	for name, cfg := range configs {
		for _, skipAuth := range []bool{false, true} {
			label := name
			if skipAuth {
				label += " (unauthenticated)"
			}

			t.Run(label, func(t *testing.T) {
				t.Parallel()

				client, err := kuma.BuildForTest(context.Background(), cfg, skipAuth)
				if err != nil {
					t.Fatalf("building the client: %s", err)
				}
				if !client.HasBaseContextForTest() {
					t.Error("the constructor left the base context nil, which makes the " +
						"first dial panic")
				}
			})
		}
	}
}

// Why the field exists: deriving it from the caller would tear the connection
// down after the first resource.
func TestTheBaseContextOutlivesTheCallersContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())

	client, err := kuma.BuildForTest(ctx, kuma.Config{Endpoint: "http://127.0.0.1:1"}, false)
	if err != nil {
		t.Fatalf("building the client: %s", err)
	}

	cancel() // as Terraform does at the end of an operation

	if !client.HasBaseContextForTest() {
		t.Fatal("the base context disappeared")
	}

	// This fails because the port is closed, not because the context was cancelled.
	if _, err := client.ListMonitors(context.Background()); err == nil {
		t.Error("expected a connection failure against a closed port")
	}
}

// Without this the failure surfaces as an unparseable URL during a dial, which
// reads like a network problem.
func TestAnEmptyEndpointIsRejectedBeforeDialing(t *testing.T) {
	t.Parallel()

	_, err := kuma.BuildForTest(context.Background(), kuma.Config{}, false)
	if err == nil {
		t.Fatal("an empty endpoint should be rejected")
	}
	if !strings.Contains(err.Error(), "endpoint") {
		t.Errorf("the message should name the missing setting: %s", err)
	}
}

// A zero timeout makes every RPC give up immediately.
func TestTheConstructorNormalizesTimeoutAndRetries(t *testing.T) {
	t.Parallel()

	client, err := kuma.BuildForTest(context.Background(), kuma.Config{
		Endpoint:   "http://127.0.0.1:1",
		Timeout:    0,
		MaxRetries: -5,
	}, false)
	if err != nil {
		t.Fatalf("building the client: %s", err)
	}

	if got := client.TimeoutForTest(); got != kuma.DefaultTimeout {
		t.Errorf("timeout = %s, want the default %s — a zero timeout fails every call",
			got, kuma.DefaultTimeout)
	}
	if got := client.MaxRetriesForTest(); got < 0 {
		t.Errorf("max retries = %d, want it clamped to zero or above", got)
	}
}
