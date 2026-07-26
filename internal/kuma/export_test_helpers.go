package kuma

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/url"
	"time"
)

// Hooks used only by tests, to reach code paths a live server will not produce on
// demand: a malformed acknowledgement, or a cache that has to be refetched.

// ConnectBackoffForTest exposes the reconnect backoff schedule. The schedule is
// worth pinning down: it is what keeps a rate-limited login from burning every
// retry inside one refill window.
func ConnectBackoffForTest(attempt int, err error) time.Duration {
	return connectBackoff(attempt, err)
}

// DecodeAckForTest exposes acknowledgement decoding. A real server does not send
// malformed acknowledgements, but the decoder still has to handle them, because a
// silent mis-decode is how an entity turns invisible.
func DecodeAckForTest(event string, raw []byte, out any) error {
	return decodeAck(event, raw, out)
}

// InvalidateCachesForTest drops every cached list, so the next read has to go
// back to the server through the refresh path. Without this the refresh path is
// unreachable: the lists arrive during login and stay loaded.
func (c *Client) InvalidateCachesForTest() {
	c.cache.invalidate()
}

// NewForHTTPTestOnly builds a structurally complete client that has never
// connected, without dialing anything.
//
// Two things need it. The reads that go over HTTP instead of Socket.IO — only
// GetStatusPageGroups today — where pointing at an httptest server is the only
// way to cover a 404, a 500 or a malformed body, none of which a healthy Uptime
// Kuma produces. And any check that every method fails cleanly instead of
// hanging, since a client that never connected makes every call take its error
// path.
func NewForHTTPTestOnly(endpoint string) *Client {
	return &Client{
		cfg:     Config{Endpoint: endpoint, Timeout: DefaultTimeout},
		cache:   newCache(),
		baseCtx: context.Background(),
	}
}

// BuildForTest runs the constructor without dialing, so the invariants it sets up
// can be inspected. The real constructors cannot be used for this: they return
// nil unless a server answers.
func BuildForTest(ctx context.Context, cfg Config, skipAuth bool) (*Client, error) {
	return buildClient(ctx, cfg, skipAuth)
}

// NewWithoutBaseContextForTest builds the one shape a client must never be in: no
// base context. Dialing derives the connection context from it, so a nil one used
// to panic and take the provider process down with it.
//
// This exists only so the guard against that can be tested. Nothing in the
// provider should produce a client like this — see TestEveryClientHasABaseContext.
func NewWithoutBaseContextForTest(endpoint string) *Client {
	return &Client{
		cfg:   Config{Endpoint: endpoint, Timeout: DefaultTimeout},
		cache: newCache(),
	}
}

// HasBaseContextForTest reports whether the base context was set.
func (c *Client) HasBaseContextForTest() bool { return c.baseCtx != nil }

// TimeoutForTest and MaxRetriesForTest expose the normalized configuration, so a
// test can check the constructor's defaults were applied.
func (c *Client) TimeoutForTest() time.Duration { return c.cfg.Timeout }

// MaxRetriesForTest exposes the clamped retry count.
func (c *Client) MaxRetriesForTest() int { return c.cfg.MaxRetries }

// SessionForTest mirrors the socketSession interface, so a test can supply its
// own answers to emitted events.
//
// This is the hook that makes the wire protocol testable without a server. A
// healthy Uptime Kuma cannot be asked to acknowledge with ok:true and no ID, to
// return a null object, or to answer with a malformed payload — yet every one of
// those has a branch in this package, because the alternative is writing a zero
// ID into Terraform state.
type SessionForTest interface {
	Emit(event any, args ...any) error
	Close() error
}

// InjectSessionForTest installs a session and marks the client authenticated, so
// calls go straight to it instead of dialing.
func (c *Client) InjectSessionForTest(session SessionForTest) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sio = session
	c.healthy = true
	c.jwt = "test-token"
}

// IsHealthyForTest reports whether the client still trusts its session. Several
// failures are supposed to drop it so the next call reconnects, and that is
// invisible from the outside otherwise.
func (c *Client) IsHealthyForTest() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.healthy
}

// SeedCachesForTest marks the pushed lists loaded, so a read hits the cache
// instead of waiting for a push that no fake session will send.
func (c *Client) SeedCachesForTest() {
	c.cache.notifications.replace(map[int]Notification{})
	c.cache.proxies.replace(map[int]Proxy{})
	c.cache.dockerHosts.replace(map[int]DockerHost{})
	c.cache.remoteBrowsers.replace(map[int]RemoteBrowser{})
}

// SetTimeoutForTest shortens the RPC timeout. Tests that deliberately let a call
// go unanswered would otherwise wait the full default.
func (c *Client) SetTimeoutForTest(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cfg.Timeout = d
}

// TLSWebSocketForTest exposes the custom WebSocket so its undialed state can be
// exercised. A live connection never goes through those branches, but a failed or
// pending dial does, and the alternative to returning an error there is a nil
// dereference that kills the plugin process.
type TLSWebSocketForTest = tlsWebSocket

// NewTLSWebSocketForTest builds one that has not dialed.
func NewTLSWebSocketForTest(tlsConfig *tls.Config) *TLSWebSocketForTest {
	return &tlsWebSocket{tlsConfig: tlsConfig}
}

// DialForTest drives Dial with raw strings, so a test does not have to build URLs.
func DialForTest(w *TLSWebSocketForTest, rawURL, rawOrigin string) error {
	target, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	origin, err := url.Parse(rawOrigin)
	if err != nil {
		return err
	}
	return w.Dial(context.Background(), target, origin)
}

// HTTPClientForTest exposes the HTTP client used by the long-polling transport,
// which carries the Engine.IO handshake before the upgrade to WebSocket.
func HTTPClientForTest(tlsConfig *tls.Config) *http.Client {
	return httpClient(tlsConfig)
}

// AuthenticateForTest drives the login sequence against a supplied session.
//
// Authentication is where the most consequential branches live — reusing a cached
// token, falling back to a password when the server rejects it, refusing to
// proceed when 2FA is required — and none of them are reachable through the
// public API, which only exposes "it connected" or "it did not".
func (c *Client) AuthenticateForTest(ctx context.Context, session SessionForTest) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.authenticateLocked(ctx, session)
}

// SetTokenForTest seeds the cached JWT, as a previous successful login would.
func (c *Client) SetTokenForTest(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.jwt = token
}

// TokenForTest returns the cached JWT.
func (c *Client) TokenForTest() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.jwt
}

// SetCredentialsForTest replaces the configured credentials.
func (c *Client) SetCredentialsForTest(username, password, totp string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cfg.Username = username
	c.cfg.Password = password
	c.cfg.TOTPToken = totp
}

// SetSkipAuthForTest toggles the unauthenticated mode used by needSetup/setup.
func (c *Client) SetSkipAuthForTest(skip bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.skipAuth = skip
}
