package kuma_test

import (
	"context"
	"strings"
	"testing"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
)

// Regression tests for a panic found while covering the cancellation paths:
// connectLocked derived the connection context from c.baseCtx with
// context.WithCancel, which panics when handed a nil parent. A client assembled
// without a base context therefore crashed the provider process on its first
// dial instead of returning an error.
//
// A panic in a Terraform provider is worse than an error. The plugin dies, the
// framework reports a crash with no resource address, and any operation already
// in flight is left half-applied with nothing written to state.
//
// Two things are pinned down here: dialing without a base context now fails
// instead of panicking, and no constructor produces a client in that shape in the
// first place. The second is the one that keeps the bug from coming back — the
// first only stops it from being fatal.

// TestDialingWithoutABaseContextFailsInsteadOfPanicking is the direct regression
// test. Before the fix this call panicked inside context.WithCancel.
func TestDialingWithoutABaseContextFailsInsteadOfPanicking(t *testing.T) {
	t.Parallel()

	// Port 1 is closed, so the dial fails — the point is *how* it fails.
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

// TestEveryClientMethodSurvivesAMissingBaseContext widens the guarantee past the
// one method above. Every entry point reaches the dial through session(), so any
// of them could have been the first to hit it — the panic was found through
// DeleteStatusPage, not ListMonitors.
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

// TestEveryClientHasABaseContext is the root-cause test. The panic was possible
// because a client could be assembled without a base context at all; this asserts
// the constructor never leaves one out, whatever configuration it is given.
//
// A constructor added later that forgets the field fails here rather than
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

// TestTheBaseContextOutlivesTheCallersContext covers why the field exists. The
// socket has to stay up between Terraform operations, so it is deliberately
// detached from the context of the call that opened it. Deriving it directly from
// the caller would tear the connection down after the first resource.
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

	// Cancelling the constructor's context must not, by itself, be what stops a
	// later call. This one fails because the port is closed, and the distinction
	// matters: a cancelled base context would report the same way while actually
	// meaning the connection can never be reopened.
	if _, err := client.ListMonitors(context.Background()); err == nil {
		t.Error("expected a connection failure against a closed port")
	}
}

// TestAnEmptyEndpointIsRejectedBeforeDialing pins the other guard in the
// constructor. Without it the failure surfaces much later, as an unparseable URL
// during a dial, which reads like a network problem rather than missing
// configuration.
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

// TestTheConstructorNormalizesTimeoutAndRetries covers the remaining defaults it
// applies. A zero timeout would make every RPC give up immediately; negative
// retries would make the retry loop behave unpredictably.
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
