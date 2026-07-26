package kuma_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/maldikhan/go.socket.io/socket.io/v5/client/emit"
)

// Regressions. Each was silent, so each test names what breaks.

// go.socket.io only invokes func([]any) directly; any other signature takes its
// reflection path and gets dropped without a word.
func TestTheAckCallbackSignatureIsTheOneTheLibraryInvokes(t *testing.T) {
	t.Parallel()

	inspector := &signatureInspector{}
	client := kuma.NewForHTTPTestOnly("http://127.0.0.1:1")
	client.SetTimeoutForTest(150 * time.Millisecond)
	client.InjectSessionForTest(inspector)

	// The inspector never answers, so the context ends the call.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, _ = client.GetMonitor(ctx, 1)

	if inspector.callback == nil {
		t.Fatal("no ack callback was registered, so no acknowledgement could ever " +
			"be delivered")
	}

	want := reflect.TypeOf(func([]any) {})
	got := reflect.TypeOf(inspector.callback)
	if got != want {
		t.Errorf("the ack callback is %s, and go.socket.io only invokes %s directly.\n"+
			"Any other signature takes its reflection path, which requires every "+
			"argument to be a json.RawMessage and SILENTLY DROPS the callback when "+
			"it is not — every call then waits out its timeout with no indication why.",
			got, want)
	}
}

// Without a timeout callback, an unanswered event waits forever.
func TestTheTimeoutCallbackIsRegisteredToo(t *testing.T) {
	t.Parallel()

	inspector := &signatureInspector{}
	client := kuma.NewForHTTPTestOnly("http://127.0.0.1:1")
	client.SetTimeoutForTest(150 * time.Millisecond)
	client.InjectSessionForTest(inspector)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	_, _ = client.GetMonitor(ctx, 1)

	if inspector.timeout == nil {
		t.Error("no timeout callback was registered: an unanswered event would wait " +
			"forever rather than failing")
	}
	if inspector.timeoutDuration == nil {
		t.Fatal("no timeout duration was set")
	}
	if *inspector.timeoutDuration <= 0 {
		t.Errorf("timeout = %s, which would expire immediately", *inspector.timeoutDuration)
	}
}

// signatureInspector records the emit options instead of answering.
type signatureInspector struct {
	callback        any
	timeout         func()
	timeoutDuration *time.Duration
}

func (s *signatureInspector) Emit(_ any, args ...any) error {
	options := &emit.EmitOptions{}
	for _, arg := range args {
		if option, isOption := arg.(emit.EmitOption); isOption {
			option(options)
		}
	}
	s.callback = options.AckCallback()
	s.timeout = options.TimeoutCallback()
	s.timeoutDuration = options.Timeout()
	return nil
}

func (s *signatureInspector) Close() error { return nil }

// bean.import() turns every key present into a column in the INSERT, so a key
// the running version lacks fails the whole statement.
func TestOptionalMonitorFieldsAreOmittedNotNulled(t *testing.T) {
	t.Parallel()

	monitor := kuma.Monitor{
		Name:     "minimal",
		Type:     "http",
		URL:      strPtr("https://example.com"),
		Interval: 60,
	}
	kuma.NormalizeMonitor(&monitor)

	encoded, err := json.Marshal(monitor)
	if err != nil {
		t.Fatalf("encoding: %s", err)
	}

	var sent map[string]any
	if err := json.Unmarshal(encoded, &sent); err != nil {
		t.Fatalf("decoding: %s", err)
	}

	for key, value := range sent {
		if value == nil {
			t.Errorf("%q was sent as null. On create the server turns every key into a "+
				"column in its INSERT, so a version without that column fails the whole "+
				"statement — add `omitempty` to this field.", key)
		}
	}

	// `conditions` is excluded: the handler stringifies it unconditionally, so
	// NormalizeMonitor always fills it.
	for _, laterAddition := range []string{
		"bearer_token", "snmpOid", "snmpVersion", "rabbitmqNodes",
		"kafkaProducerBrokers", "remote_browser", "screenshot_delay",
	} {
		if _, present := sent[laterAddition]; present {
			t.Errorf("%q was sent by a monitor that does not use it; versions before it "+
				"was added have no such column and reject the insert", laterAddition)
		}
	}
}

// Why `omitempty` cannot go on everything: the server dereferences these six.
func TestRequiredMonitorFieldsAreAlwaysSent(t *testing.T) {
	t.Parallel()

	monitor := kuma.Monitor{Name: "minimal", Type: "http", URL: strPtr("https://x"), Interval: 60}
	kuma.NormalizeMonitor(&monitor)

	encoded, err := json.Marshal(monitor)
	if err != nil {
		t.Fatalf("encoding: %s", err)
	}
	var sent map[string]any
	if err := json.Unmarshal(encoded, &sent); err != nil {
		t.Fatalf("decoding: %s", err)
	}

	required := map[string]string{
		"name":                 "the server has no default",
		"type":                 "the server has no default",
		"interval":             "zero is rejected by validate()",
		"retryInterval":        "no server-side default, and zero is rejected",
		"accepted_statuscodes": "dereferenced with no nil check",
		"notificationIDList":   "an empty object is how links are removed, so it must never be omitted",
	}
	for field, why := range required {
		if _, present := sent[field]; !present {
			t.Errorf("%q was omitted, but %s", field, why)
		}
	}
}

// Each type sets a different subset; none may ride along on the others.
func TestOptionalFieldsAreOmittedForEveryMonitorType(t *testing.T) {
	t.Parallel()

	for _, wireType := range []string{
		"http", "keyword", "json-query", "ping", "port", "dns", "push", "group", "docker",
	} {
		t.Run(wireType, func(t *testing.T) {
			t.Parallel()

			monitor := kuma.Monitor{Name: "m", Type: wireType, Interval: 60}
			kuma.NormalizeMonitor(&monitor)

			encoded, err := json.Marshal(monitor)
			if err != nil {
				t.Fatalf("encoding: %s", err)
			}
			var sent map[string]any
			if err := json.Unmarshal(encoded, &sent); err != nil {
				t.Fatalf("decoding: %s", err)
			}

			for key, value := range sent {
				if value == nil {
					t.Errorf("%q sent as null on a %s monitor; add `omitempty`", key, wireType)
				}
			}
		})
	}
}

// The exemptions are the point: a field a user can clear has to be sent as null,
// which beats `omitempty` for those. The list is closed, so a new field without
// the tag fails here.
func TestOptionalFieldsAreOmittedForTheOtherEntities(t *testing.T) {
	t.Parallel()

	// field -> why null, not omission.
	nullable := map[string]string{
		"username": "clearing a proxy's credentials means sending null; omitting it would " +
			"write the old user back",
		"password": "same as username — this is how proxy authentication is removed",
		"expires": "an API key can be changed from expiring to never expiring, and null " +
			"is what that looks like",
	}

	payloads := map[string]any{
		"proxy":          kuma.Proxy{Host: "h", Port: 8080, Protocol: "http"},
		"docker host":    kuma.DockerHost{Name: "n", DockerType: "socket", DockerDaemon: "/var/run/docker.sock"},
		"remote browser": kuma.RemoteBrowser{Name: "n", URL: "ws://x"},
		"api key":        kuma.APIKey{Name: "n"},
		"tag":            kuma.Tag{Name: "n", Color: "#fff"},
	}

	for name, payload := range payloads {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			encoded, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("encoding: %s", err)
			}
			var sent map[string]any
			if err := json.Unmarshal(encoded, &sent); err != nil {
				t.Fatalf("decoding: %s", err)
			}

			for key, value := range sent {
				if value != nil {
					continue
				}
				if _, exempt := nullable[key]; exempt {
					continue
				}
				t.Errorf("%q was sent as null and is not one of the fields that need to "+
					"be. The server builds its INSERT from the keys present, so a version "+
					"without that column rejects the statement — add `omitempty`, or add "+
					"the field to the exemption list with the reason a user has to be able "+
					"to clear it.", key)
			}

			if id, present := sent["id"]; present {
				if number, isNumber := id.(float64); isNumber && number == 0 {
					t.Error("id was sent as 0 on a create; it has to be omitted so the " +
						"server assigns one")
				}
			}
		})
	}
}

// What earns those exemptions: the values really can be cleared.
func TestTheNullableFieldsCanActuallyBeCleared(t *testing.T) {
	t.Parallel()

	t.Run("proxy credentials", func(t *testing.T) {
		t.Parallel()

		session := newFakeSession(map[string]string{"addProxy": `{"ok":true,"id":2}`})
		client := clientWith(t, session)

		// Had authentication, no longer does.
		id := 2
		if _, err := client.SaveProxy(context.Background(), &id, kuma.Proxy{
			Host: "h", Port: 8080, Protocol: "http", Auth: kuma.Bool(false),
		}); err != nil {
			t.Fatalf("saving: %s", err)
		}

		encoded, err := json.Marshal(session.payloads["addProxy"][0])
		if err != nil {
			t.Fatalf("encoding: %s", err)
		}
		var sent map[string]any
		if err := json.Unmarshal(encoded, &sent); err != nil {
			t.Fatalf("decoding: %s", err)
		}

		for _, field := range []string{"username", "password"} {
			value, present := sent[field]
			if !present {
				t.Errorf("%q was omitted, so the server keeps the credentials the proxy "+
					"had before", field)
				continue
			}
			if value != nil {
				t.Errorf("%q = %v, want null to clear it", field, value)
			}
		}
	})

	t.Run("api key expiry", func(t *testing.T) {
		t.Parallel()

		session := newFakeSession(map[string]string{
			"addAPIKey": `{"ok":true,"keyID":4,"key":"uk4_secret"}`,
		})
		client := clientWith(t, session)

		if _, _, err := client.CreateAPIKey(context.Background(), kuma.APIKey{
			Name: "never expires",
		}); err != nil {
			t.Fatalf("creating: %s", err)
		}

		encoded, err := json.Marshal(session.payloads["addAPIKey"][0])
		if err != nil {
			t.Fatalf("encoding: %s", err)
		}
		var sent map[string]any
		if err := json.Unmarshal(encoded, &sent); err != nil {
			t.Fatalf("decoding: %s", err)
		}

		value, present := sent["expires"]
		if !present {
			t.Fatal("expires was omitted; a key with no expiry needs it sent as null")
		}
		if value != nil {
			t.Errorf("expires = %v, want null for a key that never expires", value)
		}
	})
}

// 20 logins per minute, server-wide, and every client construction spends one.
func TestSharedSessionsAreReusedForTheSameConfiguration(t *testing.T) {
	kuma.ResetPoolForTest()
	t.Cleanup(kuma.ResetPoolForTest)

	cfg := kuma.Config{
		Endpoint: "http://127.0.0.1:1",
		Username: "admin",
		Password: "secret",
	}

	seeded := kuma.NewForHTTPTestOnly(cfg.Endpoint)
	kuma.SeedPoolForTest(cfg, seeded)

	// Nothing listens on port 1, so dialing instead of reusing would fail here.
	got, err := kuma.Shared(context.Background(), cfg)
	if err != nil {
		t.Fatalf("an identical configuration should reuse the open session, not dial: %s", err)
	}
	if got != seeded {
		t.Error("a second configuration with the same values opened a new session; " +
			"the server allows only 20 logins a minute across all clients")
	}
}

// Rotating a password has to force a fresh login.
func TestSharedSessionsAreNotReusedWhenTheCredentialsChange(t *testing.T) {
	t.Parallel()

	base := kuma.Config{Endpoint: "http://kuma.example.com", Username: "admin", Password: "secret"}

	differs := map[string]kuma.Config{
		"a different endpoint": {Endpoint: "http://other.example.com", Username: "admin", Password: "secret"},
		"a different user":     {Endpoint: "http://kuma.example.com", Username: "other", Password: "secret"},
		"a rotated password":   {Endpoint: "http://kuma.example.com", Username: "admin", Password: "rotated"},
		"tls verification off": {
			Endpoint: "http://kuma.example.com", Username: "admin", Password: "secret",
			InsecureSkipVerify: true,
		},
	}

	baseKey := kuma.PoolKeyForTest(base)
	for name, cfg := range differs {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if kuma.PoolKeyForTest(cfg) == baseKey {
				t.Errorf("%s shares a pool key with the base configuration, so it would "+
					"be handed a session established with different settings", name)
			}
		})
	}
}

// Differing in something the session does not depend on must still share it.
func TestSharedSessionsMatchOnEverythingThatMatters(t *testing.T) {
	t.Parallel()

	base := kuma.Config{Endpoint: "http://kuma.example.com", Username: "admin", Password: "secret"}

	same := map[string]kuma.Config{
		"a different timeout": {
			Endpoint: base.Endpoint, Username: base.Username, Password: base.Password,
			Timeout: 90 * time.Second,
		},
		"a different retry count": {
			Endpoint: base.Endpoint, Username: base.Username, Password: base.Password,
			MaxRetries: 10,
		},
	}

	baseKey := kuma.PoolKeyForTest(base)
	for name, cfg := range same {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if kuma.PoolKeyForTest(cfg) != baseKey {
				t.Errorf("%s produced a different pool key, so it would spend another "+
					"login for a session it could have shared", name)
			}
		})
	}
}

// Caching a failed login would hand the same broken session to every caller.
func TestAFailedConnectionIsNotPooled(t *testing.T) {
	kuma.ResetPoolForTest()
	t.Cleanup(kuma.ResetPoolForTest)

	cfg := kuma.Config{
		Endpoint:   "http://127.0.0.1:1", // closed
		Username:   "admin",
		Password:   "secret",
		Timeout:    100 * time.Millisecond,
		MaxRetries: 0,
	}

	if _, err := kuma.Shared(context.Background(), cfg); err == nil {
		t.Fatal("connecting to a closed port should fail")
	}

	_, err := kuma.Shared(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected the same failure")
	}
	if strings.Contains(err.Error(), "cached") {
		t.Errorf("the failure looks like it came from the pool: %s", err)
	}
}

// A create must wait for the push that carries its own row.
//
// Terraform applies up to ten resources at once over the one shared session, so
// the first push after a write is often another write's, and its list predates
// this row. Returning on it makes the Read that follows report the resource as
// missing — which is what `uptimekuma_notification` did in the demo.
func TestACreateWaitsForThePushCarryingItsOwnRow(t *testing.T) {
	t.Parallel()

	client := kuma.NewForHTTPTestOnly("http://127.0.0.1:1")
	client.SetTimeoutForTest(5 * time.Second)
	session := &interleavedPushSession{client: client}
	client.InjectSessionForTest(session)

	id, err := client.SaveNotification(context.Background(), nil, map[string]any{"name": "mine"})
	if err != nil {
		t.Fatalf("SaveNotification: %v", err)
	}

	if !client.Cache().Notifications.Has(id) {
		t.Fatalf("the create returned before the pushed list held id %d.\n"+
			"It woke on another write's push, whose list predates this row, so the "+
			"Read that follows finds nothing and Terraform reports the resource as "+
			"missing right after creating it.", id)
	}
}

// interleavedPushSession answers a create the way a busy server does: another
// resource's list lands first, and the one holding this row only afterwards.
type interleavedPushSession struct{ client *kuma.Client }

func (s *interleavedPushSession) Emit(_ any, args ...any) error {
	callback := findAckCallback(args)
	if callback == nil {
		return nil
	}
	notifications := s.client.Cache().Notifications
	go func() {
		notifications.Replace(map[int]kuma.Notification{1: {ID: 1}})
		callback([]any{json.RawMessage(`{"ok":true,"id":2}`)})

		time.Sleep(50 * time.Millisecond)
		notifications.Replace(map[int]kuma.Notification{1: {ID: 1}, 2: {ID: 2}})
	}()
	return nil
}

func (s *interleavedPushSession) Close() error { return nil }

// No call may outlive the configured timeout, whatever the library does with it.
//
// go.socket.io runs one goroutine per acknowledgement, selecting on the ack, its
// timer and its own client context. The context branch only logs: neither the ack
// callback nor the timeout callback fires. Closing a session cancels exactly that
// context, so a reconnect forced by another goroutine — which the provider does
// on purpose, to make the server resend the push-only lists — leaves this call
// waiting on a channel nothing will ever write to. Terraform's apply context
// carries no deadline, so that wait was forever.
func TestACallCannotOutliveItsTimeout(t *testing.T) {
	t.Parallel()

	client := kuma.NewForHTTPTestOnly("http://127.0.0.1:1")
	client.SetTimeoutForTest(150 * time.Millisecond)
	// Answers with neither an acknowledgement nor a timeout, as a session whose
	// context was cancelled underneath it does.
	client.InjectSessionForTest(&silentSession{})

	done := make(chan error, 1)
	go func() {
		_, err := client.GetMonitor(context.Background(), 1)
		done <- err
	}()

	select {
	case err := <-done:
		if !errors.Is(err, kuma.ErrTimeout) {
			t.Errorf("the call should end as a timeout, so RetryRPC reconnects: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the call never ended.\n" +
			"Nothing bounds it but the caller's context, and Terraform's apply " +
			"context has no deadline — so the resource hangs until the operator " +
			"kills terraform, with the plugin still alive.")
	}
}

// silentSession registers the callbacks and never invokes either.
type silentSession struct{}

func (s *silentSession) Emit(_ any, _ ...any) error { return nil }
func (s *silentSession) Close() error               { return nil }

// A reconnect must not tear a session down while a call is still waiting on it.
//
// The provider forces reconnects on purpose — it is the only way to make the
// server resend the push-only lists — and Terraform drives ten resources over
// the one shared session, so calls in flight during a reconnect are normal. The
// close cancels the library's client context, which drops those calls' ack
// callbacks without a word.
func TestAReconnectWaitsForTheCallsInFlight(t *testing.T) {
	t.Parallel()

	client := kuma.NewForHTTPTestOnly("http://127.0.0.1:1")
	client.SetTimeoutForTest(5 * time.Second)
	session := &slowSession{emitting: make(chan struct{})}
	client.InjectSessionForTest(session)

	called := make(chan error, 1)
	go func() {
		_, err := client.GetMonitor(context.Background(), 1)
		called <- err
	}()

	<-session.emitting

	// Dialing 127.0.0.1:1 fails, but only after the close this test is about.
	reconnected := make(chan struct{})
	go func() {
		defer close(reconnected)
		client.ForceReconnectForTest(context.Background())
	}()

	select {
	case err := <-called:
		if err != nil {
			t.Fatalf("the call should have been acknowledged: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the call never finished")
	}
	<-reconnected

	if session.closedWhileWaiting() {
		t.Error("the session was closed while a call was still waiting for its " +
			"acknowledgement.\nClosing cancels the library's client context, and its " +
			"per-ack goroutine then fires neither the ack callback nor the timeout " +
			"one — the call is left waiting on a channel nothing will write to.")
	}
}

// slowSession answers after a delay, and records whether it was closed before it
// got the chance.
type slowSession struct {
	emitting chan struct{}

	mu       sync.Mutex
	waiting  bool
	closedIn bool
}

func (s *slowSession) Emit(_ any, args ...any) error {
	callback := findAckCallback(args)

	s.mu.Lock()
	s.waiting = true
	s.mu.Unlock()
	close(s.emitting)

	go func() {
		time.Sleep(200 * time.Millisecond)
		s.mu.Lock()
		s.waiting = false
		s.mu.Unlock()
		if callback != nil {
			callback([]any{json.RawMessage(`{"ok":true,"monitor":{"id":1}}`)})
		}
	}()
	return nil
}

func (s *slowSession) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.waiting {
		s.closedIn = true
	}
	return nil
}

func (s *slowSession) closedWhileWaiting() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closedIn
}

func strPtr(s string) *string { return &s }
