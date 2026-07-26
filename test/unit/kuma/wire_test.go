package kuma_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/maldikhan/go.socket.io/socket.io/v5/client/emit"
)

// Acknowledgements a healthy server never sends.
//
// Every write in this package reads an ID or an object back out of the
// acknowledgement, and each one guards against it being absent. That guard cannot
// be reached against a real Uptime Kuma: it always answers with the row it just
// wrote. But if it ever did not — a version skew, a handler changed upstream, a
// proxy truncating the payload — the alternative to the guard is writing id 0 or
// an empty object into Terraform state, which is unrecoverable without hand
// editing the state file.
//
// The fake session below answers emitted events with whatever the test wants,
// which is what makes those branches reachable.

// fakeSession answers events from a table. Anything not in the table gets a bare
// {"ok":true}, which is what most events that carry no payload reply with.
type fakeSession struct {
	replies map[string]string
	// emitErr, when set, makes Emit itself fail — the transport refusing the
	// write rather than the server rejecting it.
	emitErr error
	// timeout makes the session accept the event and never acknowledge it.
	timeout bool
	// seen records the events emitted, in order.
	seen []string
	// payloads records the arguments of the last emit of each event.
	payloads map[string][]any
	closed   bool
}

func newFakeSession(replies map[string]string) *fakeSession {
	return &fakeSession{replies: replies, payloads: map[string][]any{}}
}

func (f *fakeSession) Emit(event any, args ...any) error {
	name, _ := event.(string)
	f.seen = append(f.seen, name)

	options := &emit.EmitOptions{}
	var payloads []any
	for _, arg := range args {
		if option, isOption := arg.(emit.EmitOption); isOption {
			option(options)
			continue
		}
		payloads = append(payloads, arg)
	}
	f.payloads[name] = payloads

	if f.emitErr != nil {
		return f.emitErr
	}

	if f.timeout {
		// The library calls this when the ack does not arrive in time.
		if onTimeout := options.TimeoutCallback(); onTimeout != nil {
			go func() {
				time.Sleep(time.Millisecond)
				onTimeout()
			}()
		}
		return nil
	}

	reply, known := f.replies[name]
	if !known {
		reply = `{"ok":true}`
	}

	// The provider registers func([]any); any other signature would be dropped by
	// the library, which is the bug this shape guards against.
	callback, ok := options.AckCallback().(func([]any))
	if !ok {
		return nil
	}
	go callback([]any{json.RawMessage(reply)})
	return nil
}

func (f *fakeSession) Close() error {
	f.closed = true
	return nil
}

// clientWith returns a client wired to a fake session, without dialing.
func clientWith(t *testing.T, session kuma.SessionForTest) *kuma.Client {
	t.Helper()

	client := kuma.NewForHTTPTestOnly("http://127.0.0.1:1")
	// Short, because a few of these deliberately never answer, and mutations wait
	// for a pushed list the fake session does not send.
	client.SetTimeoutForTest(150 * time.Millisecond)
	client.InjectSessionForTest(session)
	return client
}

// TestWritesRejectAnAcknowledgementWithNoID covers the guard on every create.
// Uptime Kuma confirms with ok:true and the new row's ID; without the guard an
// answer missing the ID would be written to state as id 0, and every later
// operation would address the wrong row.
func TestWritesRejectAnAcknowledgementWithNoID(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		event string
		// reply is an acknowledgement that succeeds but carries no ID.
		reply string
		call  func(*kuma.Client, context.Context) error
	}{
		"monitor": {
			event: "add", reply: `{"ok":true}`,
			call: func(c *kuma.Client, ctx context.Context) error {
				_, err := c.CreateMonitor(ctx, kuma.Monitor{Name: "n", Type: "http"})
				return err
			},
		},
		"tag": {
			event: "addTag", reply: `{"ok":true}`,
			call: func(c *kuma.Client, ctx context.Context) error {
				_, err := c.CreateTag(ctx, kuma.Tag{Name: "n", Color: "#fff"})
				return err
			},
		},
		"tag with a zero id": {
			event: "addTag", reply: `{"ok":true,"tag":{"id":0,"name":"n"}}`,
			call: func(c *kuma.Client, ctx context.Context) error {
				_, err := c.CreateTag(ctx, kuma.Tag{Name: "n", Color: "#fff"})
				return err
			},
		},
		"notification": {
			event: "addNotification", reply: `{"ok":true}`,
			call: func(c *kuma.Client, ctx context.Context) error {
				_, err := c.SaveNotification(ctx, nil, map[string]any{"name": "n", "type": "webhook"})
				return err
			},
		},
		"proxy": {
			event: "addProxy", reply: `{"ok":true}`,
			call: func(c *kuma.Client, ctx context.Context) error {
				_, err := c.SaveProxy(ctx, nil, kuma.Proxy{Host: "h", Port: 1, Protocol: "http"})
				return err
			},
		},
		"docker host": {
			event: "addDockerHost", reply: `{"ok":true}`,
			call: func(c *kuma.Client, ctx context.Context) error {
				_, err := c.SaveDockerHost(ctx, nil, kuma.DockerHost{
					Name: "n", DockerType: "socket", DockerDaemon: "/var/run/docker.sock",
				})
				return err
			},
		},
		"remote browser": {
			event: "addRemoteBrowser", reply: `{"ok":true}`,
			call: func(c *kuma.Client, ctx context.Context) error {
				_, err := c.SaveRemoteBrowser(ctx, nil, kuma.RemoteBrowser{Name: "n", URL: "ws://x"})
				return err
			},
		},
		"api key": {
			event: "addAPIKey", reply: `{"ok":true,"key":"uk1_secret"}`,
			call: func(c *kuma.Client, ctx context.Context) error {
				_, _, err := c.CreateAPIKey(ctx, kuma.APIKey{Name: "n"})
				return err
			},
		},
		"maintenance": {
			event: "addMaintenance", reply: `{"ok":true}`,
			call: func(c *kuma.Client, ctx context.Context) error {
				_, err := c.CreateMaintenance(ctx, kuma.Maintenance{Title: "t", Strategy: "manual"})
				return err
			},
		},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			session := newFakeSession(map[string]string{tt.event: tt.reply})
			client := clientWith(t, session)
			client.SeedCachesForTest()

			err := tt.call(client, context.Background())
			if err == nil {
				t.Fatal("an acknowledgement with no ID must be an error — otherwise id 0 " +
					"is written to state and later operations address the wrong row")
			}
		})
	}
}

// TestReadsRejectAnAcknowledgementWithNoObject is the read counterpart. A getter
// that answers ok:true with no object would otherwise be reported as an empty
// resource, and Terraform would show every attribute as changed.
func TestReadsRejectAnAcknowledgementWithNoObject(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		event string
		reply string
		call  func(*kuma.Client, context.Context) error
	}{
		"monitor": {
			event: "getMonitor", reply: `{"ok":true}`,
			call: func(c *kuma.Client, ctx context.Context) error {
				_, err := c.GetMonitor(ctx, 1)
				return err
			},
		},
		"monitor with a null object": {
			event: "getMonitor", reply: `{"ok":true,"monitor":null}`,
			call: func(c *kuma.Client, ctx context.Context) error {
				_, err := c.GetMonitor(ctx, 1)
				return err
			},
		},
		"maintenance": {
			event: "getMaintenance", reply: `{"ok":true,"maintenance":null}`,
			call: func(c *kuma.Client, ctx context.Context) error {
				_, err := c.GetMaintenance(ctx, 1)
				return err
			},
		},
		"status page": {
			event: "getStatusPage", reply: `{"ok":true,"config":null}`,
			call: func(c *kuma.Client, ctx context.Context) error {
				_, err := c.GetStatusPage(ctx, "slug")
				return err
			},
		},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			session := newFakeSession(map[string]string{tt.event: tt.reply})
			client := clientWith(t, session)

			if err := tt.call(client, context.Background()); err == nil {
				t.Fatal("a getter answering with no object must be an error, not an empty resource")
			}
		})
	}
}

// TestGetSettingsNormalizesAnAbsentDocument is the deliberate exception to the
// rule above. Settings are a key/value store, and a server with none customized
// legitimately answers with no data at all — that is an empty document, not a
// failure. Returning a nil map instead would panic the first caller that indexes
// it.
func TestGetSettingsNormalizesAnAbsentDocument(t *testing.T) {
	t.Parallel()

	for name, reply := range map[string]string{
		"no data key": `{"ok":true}`,
		"null data":   `{"ok":true,"data":null}`,
		"empty data":  `{"ok":true,"data":{}}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			session := newFakeSession(map[string]string{"getSettings": reply})
			client := clientWith(t, session)

			settings, err := client.GetSettings(context.Background())
			if err != nil {
				t.Fatalf("an empty settings document is valid, not an error: %s", err)
			}
			if settings == nil {
				t.Error("the map must never be nil: callers index it directly")
			}
			if len(settings) != 0 {
				t.Errorf("expected no settings, got %d", len(settings))
			}
		})
	}
}

// TestCreateStatusPageRequiresASlugBack covers the same guard on the one create
// whose identity is a slug rather than a number. An empty slug would become the
// Terraform ID, and the resource could never be read back.
func TestCreateStatusPageRequiresASlugBack(t *testing.T) {
	t.Parallel()

	session := newFakeSession(map[string]string{"addStatusPage": `{"ok":true,"slug":""}`})
	client := clientWith(t, session)

	if _, err := client.CreateStatusPage(context.Background(), "Title", "slug"); err == nil {
		t.Fatal("an empty slug must be rejected: it would become the resource ID")
	}
}

// TestPostIncidentRequiresTheIncidentBack covers the incident write, whose ID the
// resource needs to build its composite Terraform ID.
func TestPostIncidentRequiresTheIncidentBack(t *testing.T) {
	t.Parallel()

	session := newFakeSession(map[string]string{"postIncident": `{"ok":true,"incident":null}`})
	client := clientWith(t, session)

	_, err := client.PostIncident(context.Background(), "slug", kuma.StatusPageIncident{
		Title: "t", Content: "c",
	})
	if err == nil {
		t.Fatal("posting an incident must return the incident, or the ID is unknown")
	}
}

// TestServerRejectionsSurfaceWithTheirMessage checks the ok:false path. The
// server's own wording is the only diagnosis a user gets, so it has to survive.
func TestServerRejectionsSurfaceWithTheirMessage(t *testing.T) {
	t.Parallel()

	session := newFakeSession(map[string]string{
		"add": `{"ok":false,"msg":"Retry interval cannot be less than 1 seconds"}`,
	})
	client := clientWith(t, session)

	_, err := client.CreateMonitor(context.Background(), kuma.Monitor{Name: "n", Type: "http"})
	if err == nil {
		t.Fatal("ok:false must be an error")
	}
	if !strings.Contains(err.Error(), "Retry interval") {
		t.Errorf("the server's message should survive: %s", err)
	}
}

// TestNotFoundIsRecognizedFromTheMessage pins the detection down. Uptime Kuma has
// no distinct not-found response: a missing row makes the server dereference null
// and report a JavaScript TypeError. Terraform depends on telling that apart from
// a real failure — one removes the resource from state, the other must not.
func TestNotFoundIsRecognizedFromTheMessage(t *testing.T) {
	t.Parallel()

	session := newFakeSession(map[string]string{
		"getMonitor": `{"ok":false,"msg":"Cannot read properties of null (reading 'id')"}`,
	})
	client := clientWith(t, session)

	_, err := client.GetMonitor(context.Background(), 999)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !kuma.IsNotFound(err) {
		t.Errorf("a null dereference on the server means the row is gone, and must be "+
			"recognized as not-found so Terraform drops it from state: %s", err)
	}
}

// TestAMalformedAcknowledgementIsAnError covers a reply that is not JSON at all.
// Silence here would be the worst outcome: the caller would see a zero value and
// treat it as a successful empty read.
func TestAMalformedAcknowledgementIsAnError(t *testing.T) {
	t.Parallel()

	for name, reply := range map[string]string{
		"not json":        `<html>502 Bad Gateway</html>`,
		"wrong shape":     `["unexpected","array"]`,
		"truncated":       `{"ok":tr`,
		"bare string":     `"surprise"`,
		"number":          `42`,
		"ok wrong type":   `{"ok":"yes"}`,
		"monitor is text": `{"ok":true,"monitor":"not an object"}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			session := newFakeSession(map[string]string{"getMonitor": reply})
			client := clientWith(t, session)

			if _, err := client.GetMonitor(context.Background(), 1); err == nil {
				t.Errorf("a malformed acknowledgement (%s) must be an error, never a zero value", reply)
			}
		})
	}
}

// TestATransportFailureDropsTheSession covers the emit path failing. The session
// has to be marked unhealthy, or every later call keeps writing into a socket
// that is gone and the provider never recovers without a restart.
func TestATransportFailureDropsTheSession(t *testing.T) {
	t.Parallel()

	session := newFakeSession(nil)
	session.emitErr = errors.New("use of closed network connection")

	client := clientWith(t, session)
	if !client.IsHealthyForTest() {
		t.Fatal("the injected session should start healthy")
	}

	if _, err := client.GetMonitor(context.Background(), 1); err == nil {
		t.Fatal("a transport failure must surface")
	}
	if client.IsHealthyForTest() {
		t.Error("a failed emit must mark the session unhealthy so the next call " +
			"reconnects; otherwise the provider keeps writing into a dead socket")
	}
}

// TestATimeoutDropsTheSession covers the same recovery path for a server that
// accepts the event and never answers.
func TestATimeoutDropsTheSession(t *testing.T) {
	t.Parallel()

	session := newFakeSession(nil)
	session.timeout = true

	client := clientWith(t, session)

	_, err := client.GetMonitor(context.Background(), 1)
	if err == nil {
		t.Fatal("an unanswered event must time out rather than hang")
	}
	if !errors.Is(err, kuma.ErrTimeout) {
		t.Errorf("the error should be recognizable as a timeout: %s", err)
	}
	if client.IsHealthyForTest() {
		t.Error("a timeout means the session stopped answering, so it must be dropped")
	}
}

// TestWritesEmitTheEventsTheServerExpects pins the event names down. They are the
// one thing no test can infer and no compiler can check, and two of them are
// counter-intuitive: creating a monitor is "add", not "addMonitor", and pausing
// goes through its own event because editMonitor never writes the active column.
func TestWritesEmitTheEventsTheServerExpects(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		want  string
		reply map[string]string
		call  func(*kuma.Client, context.Context) error
	}{
		"creating a monitor": {
			want:  "add",
			reply: map[string]string{"add": `{"ok":true,"monitorID":7}`},
			call: func(c *kuma.Client, ctx context.Context) error {
				_, err := c.CreateMonitor(ctx, kuma.Monitor{Name: "n", Type: "http"})
				return err
			},
		},
		"updating a monitor": {
			want:  "editMonitor",
			reply: map[string]string{"editMonitor": `{"ok":true,"monitorID":7}`},
			call: func(c *kuma.Client, ctx context.Context) error {
				return c.UpdateMonitor(ctx, kuma.Monitor{ID: 7, Name: "n", Type: "http"})
			},
		},
		"pausing":  {want: "pauseMonitor", call: func(c *kuma.Client, ctx context.Context) error { return c.PauseMonitor(ctx, 7) }},
		"resuming": {want: "resumeMonitor", call: func(c *kuma.Client, ctx context.Context) error { return c.ResumeMonitor(ctx, 7) }},
		"deleting": {want: "deleteMonitor", call: func(c *kuma.Client, ctx context.Context) error { return c.DeleteMonitor(ctx, 7, false) }},
	}

	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			session := newFakeSession(tt.reply)
			client := clientWith(t, session)

			if err := tt.call(client, context.Background()); err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if len(session.seen) == 0 {
				t.Fatal("nothing was emitted")
			}
			if session.seen[0] != tt.want {
				t.Errorf("emitted %q, want %q", session.seen[0], tt.want)
			}
		})
	}
}

// TestCreateAlwaysSendsTheFieldsTheServerDereferences guards the two payload
// quirks that break creates outright: accepted_statuscodes is dereferenced with
// no nil check, and notificationIDList must be present even when empty, because
// omitting it is how a link removal gets lost.
func TestCreateAlwaysSendsTheFieldsTheServerDereferences(t *testing.T) {
	t.Parallel()

	session := newFakeSession(map[string]string{"add": `{"ok":true,"monitorID":7}`})
	client := clientWith(t, session)

	// A monitor with none of those fields set by the caller.
	if _, err := client.CreateMonitor(context.Background(), kuma.Monitor{
		Name: "n", Type: "http", Interval: 60,
	}); err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	payloads := session.payloads["add"]
	if len(payloads) == 0 {
		t.Fatal("the create sent no payload")
	}
	encoded, err := json.Marshal(payloads[0])
	if err != nil {
		t.Fatalf("encoding the payload: %s", err)
	}

	var sent map[string]any
	if err := json.Unmarshal(encoded, &sent); err != nil {
		t.Fatalf("decoding the payload: %s", err)
	}

	codes, present := sent["accepted_statuscodes"]
	if !present {
		t.Error("accepted_statuscodes must always be sent: the server dereferences it " +
			"with no nil check")
	}
	if list, isList := codes.([]any); isList {
		for _, entry := range list {
			if _, isString := entry.(string); !isString {
				t.Errorf("every accepted status code has to be a string, got %T", entry)
			}
		}
	}

	if _, present := sent["notificationIDList"]; !present {
		t.Error("notificationIDList must always be sent, even empty: omitting it is how " +
			"removing the last notification silently fails")
	}

	if retry, present := sent["retryInterval"]; !present {
		t.Error("retryInterval must be sent: it has no server-side default and zero is rejected")
	} else if value, isNumber := retry.(float64); isNumber && value < 1 {
		t.Errorf("retryInterval = %v, which the server rejects", value)
	}
}

// TestClosingTearsDownTheSession covers Close, which has to reach the underlying
// socket. A leaked socket keeps a login slot busy, and the server allows only 20
// logins a minute across all clients.
func TestClosingTearsDownTheSession(t *testing.T) {
	t.Parallel()

	session := newFakeSession(nil)
	client := clientWith(t, session)

	if err := client.Close(); err != nil {
		t.Fatalf("closing: %s", err)
	}
	if !session.closed {
		t.Error("Close must reach the socket, or the login slot stays busy")
	}
}

// TestMaintenancePayloadsFillTheColumnsTheServerDereferences covers the
// normalization the client applies before every maintenance write.
//
// Two columns are NOT NULL with no default, and the server copies the payload
// straight into the insert: `active`, and `dateRange`, which is indexed whatever
// the strategy is. An unset value there fails the whole statement with a
// constraint error that names a column the user never configured.
func TestMaintenancePayloadsFillTheColumnsTheServerDereferences(t *testing.T) {
	t.Parallel()

	for name, input := range map[string]kuma.Maintenance{
		"manual":              {Title: "t", Strategy: "manual"},
		"single window":       {Title: "t", Strategy: "single"},
		"recurring weekday":   {Title: "t", Strategy: "recurring-weekday"},
		"recurring monthly":   {Title: "t", Strategy: "recurring-day-of-month"},
		"recurring interval":  {Title: "t", Strategy: "recurring-interval"},
		"cron":                {Title: "t", Strategy: "cron"},
		"active already true": {Title: "t", Strategy: "manual", Active: kuma.BoolPtr(true)},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			session := newFakeSession(map[string]string{
				"addMaintenance": `{"ok":true,"maintenanceID":6}`,
			})
			client := clientWith(t, session)

			if _, err := client.CreateMaintenance(context.Background(), input); err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			payloads := session.payloads["addMaintenance"]
			if len(payloads) == 0 {
				t.Fatal("nothing was sent")
			}
			encoded, err := json.Marshal(payloads[0])
			if err != nil {
				t.Fatalf("encoding: %s", err)
			}
			var sent map[string]any
			if err := json.Unmarshal(encoded, &sent); err != nil {
				t.Fatalf("decoding: %s", err)
			}

			if sent["active"] == nil {
				t.Error("active must be sent: the column is NOT NULL with no default")
			}
			dateRange, present := sent["dateRange"]
			if !present {
				t.Error("dateRange must always be sent: the server indexes it whatever " +
					"the strategy is")
			}
			if list, isList := dateRange.([]any); isList && len(list) < 2 {
				t.Errorf("dateRange has %d entries, and the server indexes [0] and [1]", len(list))
			}
			// Recurring strategies index timeRange the same way.
			if timeRange, present := sent["timeRange"]; present {
				if list, isList := timeRange.([]any); isList && len(list) < 2 {
					t.Errorf("timeRange has %d entries, and the server indexes [0] and [1]", len(list))
				}
			}
		})
	}
}

// TestMaintenanceUpdatesAreNormalizedToo guards the same fields on the edit path.
// It is a separate call in the client, and normalizing only on create is an easy
// asymmetry to introduce.
func TestMaintenanceUpdatesAreNormalizedToo(t *testing.T) {
	t.Parallel()

	session := newFakeSession(map[string]string{"editMaintenance": `{"ok":true,"maintenanceID":6}`})
	client := clientWith(t, session)

	err := client.UpdateMaintenance(context.Background(), kuma.Maintenance{
		ID: 6, Title: "t", Strategy: "manual",
	})
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}

	encoded, err := json.Marshal(session.payloads["editMaintenance"][0])
	if err != nil {
		t.Fatalf("encoding: %s", err)
	}
	var sent map[string]any
	if err := json.Unmarshal(encoded, &sent); err != nil {
		t.Fatalf("decoding: %s", err)
	}
	if sent["active"] == nil {
		t.Error("active must be sent on update as well")
	}
	if _, present := sent["dateRange"]; !present {
		t.Error("dateRange must be sent on update as well")
	}
}
