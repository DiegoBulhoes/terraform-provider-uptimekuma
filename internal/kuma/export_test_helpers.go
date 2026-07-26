package kuma

import (
	"context"
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
