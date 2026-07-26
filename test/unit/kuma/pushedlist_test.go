package kuma_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/maldikhan/go.socket.io/socket.io/v5/client/emit"
)

// Lists that only ever arrive by push.
//
// Notifications, proxies, Docker hosts and remote browsers have no getter event.
// The server sends each list after login and again after every mutation, and this
// package caches them. That makes reading one of these a wait rather than a
// request, and the wait has to end: without a timeout a resource whose push never
// arrives hangs the whole Terraform run with no indication of which one.

// TestReadingAPushOnlyListTimesOutRatherThanHanging covers the deadline. The fake
// session answers the reconnect but never pushes anything, which is what a server
// that dropped the subscription looks like.
func TestReadingAPushOnlyListTimesOutRatherThanHanging(t *testing.T) {
	t.Parallel()

	cases := map[string]func(*kuma.Client, context.Context) error{
		"notifications": func(c *kuma.Client, ctx context.Context) error {
			_, err := c.ListNotifications(ctx)
			return err
		},
		"proxies": func(c *kuma.Client, ctx context.Context) error {
			_, err := c.ListProxies(ctx)
			return err
		},
		"docker hosts": func(c *kuma.Client, ctx context.Context) error {
			_, err := c.ListDockerHosts(ctx)
			return err
		},
		"remote browsers": func(c *kuma.Client, ctx context.Context) error {
			_, err := c.ListRemoteBrowsers(ctx)
			return err
		},
	}

	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			// A client that cannot reconnect: the reconnect is how this package asks
			// the server to resend a list it has no getter for.
			client := kuma.NewForHTTPTestOnly("http://127.0.0.1:1")
			client.SetTimeoutForTest(100 * time.Millisecond)

			done := make(chan error, 1)
			go func() { done <- call(client, context.Background()) }()

			select {
			case err := <-done:
				if err == nil {
					t.Error("expected an error rather than an empty list: reporting no " +
						"objects would make Terraform delete every resource of this type")
				}
			case <-time.After(10 * time.Second):
				t.Fatal("the read hung; a missing push must time out, not block the run")
			}
		})
	}
}

// TestAPushOnlyListIsServedFromTheCacheOnceLoaded is the other half. Once a list
// has arrived, a read must not wait again — every resource of that type reads it
// during a plan, and one round trip each would be slow for no reason.
func TestAPushOnlyListIsServedFromTheCacheOnceLoaded(t *testing.T) {
	t.Parallel()

	session := newFakeSession(nil)
	client := clientWith(t, session)
	client.SeedCachesForTest()

	start := time.Now()
	for range 5 {
		if _, err := client.ListNotifications(context.Background()); err != nil {
			t.Fatalf("reading a loaded list: %s", err)
		}
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("five cached reads took %s; they should not be waiting on anything", elapsed)
	}
	if len(session.seen) != 0 {
		t.Errorf("a loaded list should need no server round trip, emitted %v", session.seen)
	}
}

// TestInvalidatingACacheForcesAReload covers the path a reconnect takes. The
// cached lists are dropped when a session is replaced, because anything could have
// changed while the connection was gone — and reloading them has to be possible,
// which is a bug this package already had once.
func TestInvalidatingACacheForcesAReload(t *testing.T) {
	t.Parallel()

	session := newFakeSession(nil)
	client := clientWith(t, session)
	client.SeedCachesForTest()

	if _, err := client.ListNotifications(context.Background()); err != nil {
		t.Fatalf("the seeded list should read cleanly: %s", err)
	}

	client.InvalidateCachesForTest()
	client.SetTimeoutForTest(100 * time.Millisecond)

	// With the cache dropped and no push coming, this must fail rather than
	// return the stale contents or an empty list.
	if _, err := client.ListNotifications(context.Background()); err == nil {
		t.Error("after invalidation the list has to be refetched, and a failure to " +
			"do so must surface")
	}
}

// TestAGetterBackedListReportsItsFailure covers the branch for the lists that do
// have a getter event. There the refresh is a request, and its failure is what has
// to surface.
func TestAGetterBackedListReportsItsFailure(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		event string
		call  func(*kuma.Client, context.Context) error
	}{
		"monitors": {
			event: "getMonitorList",
			call: func(c *kuma.Client, ctx context.Context) error {
				_, err := c.ListMonitors(ctx)
				return err
			},
		},
		"maintenances": {
			event: "getMaintenanceList",
			call: func(c *kuma.Client, ctx context.Context) error {
				_, err := c.ListMaintenances(ctx)
				return err
			},
		},
		"api keys": {
			event: "getAPIKeyList",
			call: func(c *kuma.Client, ctx context.Context) error {
				_, err := c.ListAPIKeys(ctx)
				return err
			},
		},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			session := newFakeSession(map[string]string{
				tt.event: `{"ok":false,"msg":"Permission denied."}`,
			})
			client := clientWith(t, session)

			if err := tt.call(client, context.Background()); err == nil {
				t.Error("a rejected getter must surface, not produce an empty list")
			}
		})
	}
}

// TestADegenerateAcknowledgementIsAnError covers the two shapes the ack decoder
// cannot make sense of. The library hands the callback whatever arrived, and a
// silent mis-decode here is how an entity turns invisible: the caller sees a zero
// value and treats it as "does not exist".
func TestADegenerateAcknowledgementIsAnError(t *testing.T) {
	t.Parallel()

	t.Run("an empty acknowledgement", func(t *testing.T) {
		t.Parallel()

		session := &degenerateSession{mode: emptyAck}
		client := clientWith(t, session)

		_, err := client.GetMonitor(context.Background(), 1)
		if err == nil {
			t.Fatal("an acknowledgement with no arguments must be an error")
		}
		if !strings.Contains(err.Error(), "empty acknowledgement") {
			t.Errorf("the message should say what was wrong: %s", err)
		}
	})

	t.Run("an unexpected argument type", func(t *testing.T) {
		t.Parallel()

		session := &degenerateSession{mode: wrongType}
		client := clientWith(t, session)

		_, err := client.GetMonitor(context.Background(), 1)
		if err == nil {
			t.Fatal("an acknowledgement of the wrong type must be an error")
		}
		if !strings.Contains(err.Error(), "unexpected acknowledgement type") {
			t.Errorf("the message should name the problem: %s", err)
		}
	})
}

// TestACancelledContextInterruptsAWaitingCall covers giving up while the
// acknowledgement is still outstanding, which is what happens when the user
// interrupts Terraform.
func TestACancelledContextInterruptsAWaitingCall(t *testing.T) {
	t.Parallel()

	session := &degenerateSession{mode: neverAnswers}
	client := kuma.NewForHTTPTestOnly("http://127.0.0.1:1")
	client.SetTimeoutForTest(30 * time.Second) // long enough that the context wins
	client.InjectSessionForTest(session)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	_, err := client.GetMonitor(ctx, 1)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("a cancelled context must end the call")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("the error should report the cancellation: %s", err)
	}
	if elapsed > 5*time.Second {
		t.Errorf("the call took %s; it should have ended when the context did, not "+
			"waited out the RPC timeout", elapsed)
	}
}

// findAckCallback digs the ack callback out of the emit options. The provider
// registers func([]any) deliberately: any other signature takes the library's
// reflection path, which requires every argument to be json.RawMessage and
// silently drops the callback when it is not.
func findAckCallback(args []any) func([]any) {
	options := &emit.EmitOptions{}
	for _, arg := range args {
		if option, isOption := arg.(emit.EmitOption); isOption {
			option(options)
		}
	}
	callback, _ := options.AckCallback().(func([]any))
	return callback
}

// degenerateSession produces acknowledgements the decoder has to reject, which a
// real server never sends.
type degenerateSession struct{ mode ackMode }

type ackMode int

const (
	emptyAck ackMode = iota
	wrongType
	neverAnswers
)

func (d *degenerateSession) Emit(_ any, args ...any) error {
	callback := findAckCallback(args)
	if callback == nil {
		return nil
	}
	switch d.mode {
	case emptyAck:
		go callback(nil)
	case wrongType:
		// A string where the library normally passes json.RawMessage.
		go callback([]any{"not raw json"})
	case neverAnswers:
		// Deliberately nothing.
	}
	return nil
}

func (d *degenerateSession) Close() error { return nil }
