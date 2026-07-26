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

// Notifications, proxies, Docker hosts and remote browsers have no getter event,
// so reading one is a wait rather than a request. The wait has to end.

// What a server that dropped the subscription looks like.
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

			// A reconnect is how this package asks for a list it has no getter for.
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

// Every resource of that type reads this during a plan; one wait each is enough.
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

// A replaced session drops the cached lists, and reloading them has to work —
// this package got that wrong once.
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

	// Must fail rather than return stale contents or an empty list.
	if _, err := client.ListNotifications(context.Background()); err == nil {
		t.Error("after invalidation the list has to be refetched, and a failure to " +
			"do so must surface")
	}
}

// For the lists with a getter, the refresh is a request that can fail.
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

// A silent mis-decode here makes an entity invisible: the caller sees a zero
// value and reads it as "does not exist".
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

// Giving up while the acknowledgement is still outstanding.
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

// findAckCallback digs the ack callback out of the emit options.
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

// degenerateSession produces acknowledgements the decoder has to reject.
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
	}
	return nil
}

func (d *degenerateSession) Close() error { return nil }
