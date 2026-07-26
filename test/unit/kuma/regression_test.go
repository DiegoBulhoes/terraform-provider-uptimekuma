package kuma_test

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/maldikhan/go.socket.io/socket.io/v5/client/emit"
)

// Regression tests for three failures that cost real debugging time on this
// project. Each one was silent: nothing crashed, nothing logged, the provider
// simply did the wrong thing.
//
// They are grouped here because none of them belongs to a single method. Each is a
// property of the package as a whole, and each would come back the same way — as a
// small, reasonable-looking edit somewhere else.

// ── The ack callback signature ──────────────────────────────────────

// TestTheAckCallbackSignatureIsTheOneTheLibraryInvokes is the regression test for
// the failure that took longest to find.
//
// go.socket.io accepts `any` as its ack callback and picks how to invoke it by
// reflecting on the actual type. Exactly one signature is handed the raw
// arguments: func([]any). Everything else goes down a reflection path that
// requires every argument to be a json.RawMessage, and when that does not hold the
// library **drops the callback without a word**.
//
// The connect handler was written as func(any). Nothing failed. Nothing logged.
// The provider waited for an acknowledgement that would never be delivered, and
// every operation timed out with a message about the server not answering.
//
// This test reflects on what the package actually passes to emit.WithAck and fails
// if it is anything but func([]any), naming the consequence — because the
// alternative is another afternoon spent looking at the server.
func TestTheAckCallbackSignatureIsTheOneTheLibraryInvokes(t *testing.T) {
	t.Parallel()

	inspector := &signatureInspector{}
	client := kuma.NewForHTTPTestOnly("http://127.0.0.1:1")
	client.SetTimeoutForTest(150 * time.Millisecond)
	client.InjectSessionForTest(inspector)

	// The inspector records the options and never answers, so the call is ended by
	// the context rather than by an acknowledgement.
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

// TestTheTimeoutCallbackIsRegisteredToo covers the other half of the same emit.
// Without it a server that accepts an event and never answers leaves the call
// waiting on a channel nothing will ever write to.
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

// signatureInspector records the emit options rather than answering them.
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

// ── omitempty as a compatibility requirement ────────────────────────

// TestOptionalMonitorFieldsAreOmittedNotNulled is the regression test for
// "table monitor has no column named bearer_token" on Uptime Kuma 2.2 and 2.3.
//
// On create the server hands the payload to bean.import(), which turns every key
// present into a column in the INSERT. A key the running version does not have
// fails the whole statement — so sending "bearer_token": null to 2.2 breaks a
// monitor that has nothing to do with bearer tokens.
//
// The fix is `omitempty` on every optional field, which makes one payload work
// across 2.2 to 2.4. It is invisible in review: a new field added without the tag
// works perfectly against the newest version and breaks every older one.
func TestOptionalMonitorFieldsAreOmittedNotNulled(t *testing.T) {
	t.Parallel()

	// A monitor with only what an HTTP check needs.
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

	// Any key present with a null value is a column name sent to the server for no
	// reason, and one the running version may not have.
	for key, value := range sent {
		if value == nil {
			t.Errorf("%q was sent as null. On create the server turns every key into a "+
				"column in its INSERT, so a version without that column fails the whole "+
				"statement — add `omitempty` to this field.", key)
		}
	}

	// Fields added in later releases, spot-checked by name so the failure says
	// which one would break.
	//
	// `conditions` is deliberately not here: the handler JSON.stringify's it
	// unconditionally, so NormalizeMonitor fills it with an empty array for the
	// same reason accepted_statuscodes is filled in — the server dereferences it.
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

// TestRequiredMonitorFieldsAreAlwaysSent is the counterpart, and the reason
// `omitempty` cannot simply be applied to everything. These six the server
// dereferences or constrains, so omitting them is its own failure — documented one
// by one in CLAUDE.md.
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

// TestOptionalFieldsAreOmittedForEveryMonitorType widens the guarantee past HTTP.
// Each type sets a different subset, and a field belonging to one type must not
// ride along on the others.
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

// TestOptionalFieldsAreOmittedForTheOtherEntities covers the payloads built from
// database rows, which have the same constraint for the same reason.
//
// A handful of fields are exempt, and the exemptions are the interesting part.
// Two documented rules pull in opposite directions here:
//
//   - Optional fields need `omitempty`, because the server turns every key present
//     into a column in its INSERT.
//   - Payloads are built from the plan rather than merged onto the server's copy,
//     because a removed attribute has to reach the server as null — omitting it
//     writes the old value straight back.
//
// For a field whose value a user can clear, the second rule wins: sending null is
// the only way to empty it. That is safe precisely for the fields below, which have
// existed since 1.x, so no supported version is missing the column.
//
// The list is closed on purpose. A new optional field added without `omitempty`
// fails this test, and whoever adds it has to decide which rule applies rather than
// inheriting an exemption by accident.
func TestOptionalFieldsAreOmittedForTheOtherEntities(t *testing.T) {
	t.Parallel()

	// field -> why null has to be sent instead of the key being omitted.
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

			// id is assigned by the server on create and must not be sent as 0.
			if id, present := sent["id"]; present {
				if number, isNumber := id.(float64); isNumber && number == 0 {
					t.Error("id was sent as 0 on a create; it has to be omitted so the " +
						"server assigns one")
				}
			}
		})
	}
}

// TestTheNullableFieldsCanActuallyBeCleared is what earns those exemptions. If a
// value could not be cleared through the provider, sending null would be pointless
// and `omitempty` would be the better choice.
func TestTheNullableFieldsCanActuallyBeCleared(t *testing.T) {
	t.Parallel()

	t.Run("proxy credentials", func(t *testing.T) {
		t.Parallel()

		session := newFakeSession(map[string]string{"addProxy": `{"ok":true,"id":2}`})
		client := clientWith(t, session)

		// A proxy that had authentication and no longer does.
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

// ── The shared session pool ─────────────────────────────────────────

// TestSharedSessionsAreReusedForTheSameConfiguration is the regression test for
// the login rate limit.
//
// Uptime Kuma allows 20 logins per minute for the whole server, and every client
// construction spends one. Terraform configures a provider instance per command,
// and the acceptance framework does it several times per step — which is how a
// test run started failing with "Too frequently, try again later" on a server
// nobody else was using.
func TestSharedSessionsAreReusedForTheSameConfiguration(t *testing.T) {
	kuma.ResetPoolForTest()
	t.Cleanup(kuma.ResetPoolForTest)

	cfg := kuma.Config{
		Endpoint: "http://127.0.0.1:1",
		Username: "admin",
		Password: "secret",
	}

	// Stand in for a session a first connection established.
	seeded := kuma.NewForHTTPTestOnly(cfg.Endpoint)
	kuma.SeedPoolForTest(cfg, seeded)

	// A second configuration with the same values must get that session back
	// rather than opening another. If it dialed instead, this would fail: nothing
	// is listening on port 1.
	got, err := kuma.Shared(context.Background(), cfg)
	if err != nil {
		t.Fatalf("an identical configuration should reuse the open session, not dial: %s", err)
	}
	if got != seeded {
		t.Error("a second configuration with the same values opened a new session; " +
			"the server allows only 20 logins a minute across all clients")
	}
}

// TestSharedSessionsAreNotReusedWhenTheCredentialsChange is the limit of the
// optimization. Reuse is keyed on the credentials, so rotating a password forces a
// fresh login instead of quietly continuing on a session opened with the old one.
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

// TestSharedSessionsMatchOnEverythingThatMatters is the other direction: two
// configurations that differ only in something the session does not depend on
// should still share it, or the rate limit comes back.
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

// TestAFailedConnectionIsNotPooled covers the case that would be worse than no
// pooling at all: caching a client whose login failed would hand the same broken
// session to every later caller, and the run would fail with an error about
// something that has since been fixed.
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

	// A second attempt has to try again rather than replay the failure from a
	// cached entry.
	_, err := kuma.Shared(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected the same failure")
	}
	if strings.Contains(err.Error(), "cached") {
		t.Errorf("the failure looks like it came from the pool: %s", err)
	}
}

func strPtr(s string) *string { return &s }
