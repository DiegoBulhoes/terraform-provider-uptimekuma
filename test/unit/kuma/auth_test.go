package kuma_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
)

// The login sequence. Order matters: 20 logins per minute server-wide, and a 2FA
// code works once, so a cached JWT is tried first.

func authClient(t *testing.T, session kuma.SessionForTest, username, password, totp string) *kuma.Client {
	t.Helper()

	client := kuma.NewForHTTPTestOnly("http://127.0.0.1:1")
	client.SetTimeoutForTest(150 * time.Millisecond)
	client.SetCredentialsForTest(username, password, totp)
	client.InjectSessionForTest(session)
	return client
}

// The token has to be kept, or every reconnect spends another login.
func TestLoginStoresTheSessionToken(t *testing.T) {
	t.Parallel()

	session := newFakeSession(map[string]string{
		"login": `{"ok":true,"token":"jwt-abc"}`,
	})
	client := authClient(t, session, "admin", "secret", "")

	if err := client.AuthenticateForTest(context.Background(), session); err != nil {
		t.Fatalf("logging in: %s", err)
	}
	if got := client.TokenForTest(); got != "jwt-abc" {
		t.Errorf("token = %q, want the one the server issued", got)
	}
	if session.seen[0] != "login" {
		t.Errorf("emitted %q, want login", session.seen[0])
	}
}

// With a token in hand, the password login must not happen at all.
func TestACachedTokenIsPreferredOverThePassword(t *testing.T) {
	t.Parallel()

	session := newFakeSession(map[string]string{
		"loginByToken": `{"ok":true}`,
	})
	client := authClient(t, session, "admin", "secret", "")
	client.SetTokenForTest("jwt-cached")

	if err := client.AuthenticateForTest(context.Background(), session); err != nil {
		t.Fatalf("logging in with a cached token: %s", err)
	}

	if len(session.seen) != 1 || session.seen[0] != "loginByToken" {
		t.Errorf("emitted %v, want only loginByToken — a password login would spend "+
			"another of the 20 logins the server allows per minute", session.seen)
	}
	if got := client.TokenForTest(); got != "jwt-cached" {
		t.Errorf("the cached token should be kept, got %q", got)
	}
}

// Tokens expire; the run must not fail over a stale one we cached ourselves.
func TestARejectedTokenFallsBackToThePassword(t *testing.T) {
	t.Parallel()

	session := newFakeSession(map[string]string{
		"loginByToken": `{"ok":false,"msg":"Invalid token."}`,
		"login":        `{"ok":true,"token":"jwt-fresh"}`,
	})
	client := authClient(t, session, "admin", "secret", "")
	client.SetTokenForTest("jwt-expired")

	if err := client.AuthenticateForTest(context.Background(), session); err != nil {
		t.Fatalf("an expired token should fall back to the password, got: %s", err)
	}

	if len(session.seen) != 2 {
		t.Fatalf("emitted %v, want loginByToken then login", session.seen)
	}
	if session.seen[0] != "loginByToken" || session.seen[1] != "login" {
		t.Errorf("emitted %v, want loginByToken then login", session.seen)
	}
	if got := client.TokenForTest(); got != "jwt-fresh" {
		t.Errorf("the new token should replace the stale one, got %q", got)
	}
}

// A stale token with nothing to fall back to.
func TestARejectedTokenWithNoPasswordFails(t *testing.T) {
	t.Parallel()

	session := newFakeSession(map[string]string{
		"loginByToken": `{"ok":false,"msg":"Invalid token."}`,
	})
	client := authClient(t, session, "", "", "")
	client.SetTokenForTest("jwt-expired")

	err := client.AuthenticateForTest(context.Background(), session)
	if err == nil {
		t.Fatal("expected an error: there is no way to authenticate")
	}
	if !strings.Contains(err.Error(), "username and password") {
		t.Errorf("the message should say what is missing: %s", err)
	}
}

// Reported as a login failure, this sends the user to the wrong logs.
func TestMissingCredentialsAreReportedBeforeContactingTheServer(t *testing.T) {
	t.Parallel()

	cases := map[string]struct{ username, password string }{
		"neither":     {"", ""},
		"no password": {"admin", ""},
		"no username": {"", "secret"},
	}

	for name, credentials := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			session := newFakeSession(nil)
			client := authClient(t, session, credentials.username, credentials.password, "")

			err := client.AuthenticateForTest(context.Background(), session)
			if err == nil {
				t.Fatal("expected an error")
			}
			if len(session.seen) != 0 {
				t.Errorf("nothing should have been emitted, got %v", session.seen)
			}
		})
	}
}

// The server replies ok:true with no token, which says nothing about 2FA.
func TestATwoFactorAccountWithoutACodeIsReportedClearly(t *testing.T) {
	t.Parallel()

	session := newFakeSession(map[string]string{
		"login": `{"ok":true,"tokenRequired":true}`,
	})
	client := authClient(t, session, "admin", "secret", "")

	err := client.AuthenticateForTest(context.Background(), session)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "2FA") {
		t.Errorf("the message should mention 2FA so the user knows to set `token`: %s", err)
	}
	if !strings.Contains(err.Error(), "token") {
		t.Errorf("the message should name the attribute to set: %s", err)
	}
}

// A dropped 2FA code looks like a wrong password.
func TestATwoFactorCodeIsSentWithTheLogin(t *testing.T) {
	t.Parallel()

	session := newFakeSession(map[string]string{
		"login": `{"ok":true,"token":"jwt-abc"}`,
	})
	client := authClient(t, session, "admin", "secret", "123456")

	if err := client.AuthenticateForTest(context.Background(), session); err != nil {
		t.Fatalf("logging in: %s", err)
	}

	payloads := session.payloads["login"]
	if len(payloads) == 0 {
		t.Fatal("the login sent no payload")
	}
	sent, ok := payloads[0].(map[string]any)
	if !ok {
		t.Fatalf("expected a map payload, got %T", payloads[0])
	}
	if sent["token"] != "123456" {
		t.Errorf("token = %v, want the 2FA code — dropping it looks like a wrong password",
			sent["token"])
	}
	if sent["username"] != "admin" || sent["password"] != "secret" {
		t.Errorf("credentials did not reach the payload: %v", sent)
	}
}

// An empty token leaves the client believing it is authenticated.
func TestALoginWithNoTokenBackIsAnError(t *testing.T) {
	t.Parallel()

	for name, reply := range map[string]string{
		"no token field": `{"ok":true}`,
		"empty token":    `{"ok":true,"token":""}`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			session := newFakeSession(map[string]string{"login": reply})
			client := authClient(t, session, "admin", "secret", "")

			err := client.AuthenticateForTest(context.Background(), session)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), "token") {
				t.Errorf("the message should say the token was missing: %s", err)
			}
		})
	}
}

// Wrong password and rate limited are both ok:false; only the message differs.
func TestARejectedPasswordSurfacesTheServersMessage(t *testing.T) {
	t.Parallel()

	for name, tt := range map[string]struct{ reply, want string }{
		"wrong password": {`{"ok":false,"msg":"Incorrect username or password."}`, "Incorrect username"},
		"rate limited":   {`{"ok":false,"msg":"Too frequently, try again later."}`, "Too frequently"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			session := newFakeSession(map[string]string{"login": tt.reply})
			client := authClient(t, session, "admin", "wrong", "")

			err := client.AuthenticateForTest(context.Background(), session)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("the server's message should survive: %s", err)
			}
		})
	}
}

// The bootstrap mode: needSetup and setup work before any account exists.
func TestUnauthenticatedModeSkipsLoginEntirely(t *testing.T) {
	t.Parallel()

	session := newFakeSession(nil)
	client := authClient(t, session, "", "", "")
	client.SetSkipAuthForTest(true)

	if err := client.AuthenticateForTest(context.Background(), session); err != nil {
		t.Fatalf("unauthenticated mode should not attempt a login: %s", err)
	}
	if len(session.seen) != 0 {
		t.Errorf("nothing should have been emitted, got %v", session.seen)
	}
}

// The limit refills over a minute, so the ordinary schedule burns every attempt
// inside one window.
func TestRateLimitedLoginsBackOffMoreSlowly(t *testing.T) {
	t.Parallel()

	rateLimited := &kuma.APIError{Event: "login", Msg: "Too frequently, try again later."}
	ordinary := &kuma.APIError{Event: "login", Msg: "Incorrect username or password."}

	for attempt := 1; attempt <= 3; attempt++ {
		limited := kuma.ConnectBackoffForTest(attempt, rateLimited)
		normal := kuma.ConnectBackoffForTest(attempt, ordinary)

		if limited <= normal {
			t.Errorf("attempt %d: rate-limited wait %s is not longer than the ordinary %s; "+
				"retrying too fast burns every attempt inside one refill window",
				attempt, limited, normal)
		}
	}
}
