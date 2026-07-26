package client

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma/wire"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma/transport"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// DefaultTimeout is the per-call acknowledgement timeout when none is set.
const DefaultTimeout = 30 * time.Second

// Config holds everything needed to reach an Uptime Kuma instance.
type Config struct {
	// Endpoint is the base URL, e.g. https://kuma.example.com.
	Endpoint string
	Username string
	Password string
	// TOTPToken is the current 2FA code, required only when the account has
	// two-factor authentication enabled.
	TOTPToken string
	// Timeout bounds how long a single call waits for its acknowledgement.
	Timeout time.Duration
	// MaxRetries bounds how many times a transient failure is retried.
	MaxRetries int
	// InsecureSkipVerify disables TLS verification, for instances behind a
	// self-signed certificate.
	InsecureSkipVerify bool
}

// socketSession is the slice of the Socket.IO client this package uses. Having
// it as an interface keeps the connection logic testable without a live server.
type socketSession interface {
	Emit(event any, args ...any) error
	Close() error
}

// Client talks to Uptime Kuma over Socket.IO.
//
// The server exposes no write REST API, so every operation is an event on a
// long-lived authenticated socket. Two consequences shape this type:
//
//   - The socket outlives any single Terraform operation, so it cannot be tied
//     to a request context. baseCtx is derived with context.WithoutCancel to
//     keep the tflog logger while shedding cancellation.
//   - The library implements no reconnection, so session() re-establishes the
//     connection lazily whenever the current one is known to be broken.
type Client struct {
	cfg   Config
	cache *wire.Cache

	baseCtx context.Context

	// skipAuth is set by NewUnauthenticated, for talking to an instance that
	// has no user yet.
	skipAuth bool

	httpOnce   sync.Once
	httpClient *http.Client

	// live is held for reading by every call in flight and for writing by a
	// reconnect, so a reconnect never tears a session down underneath a call.
	// It is always taken before mu, never the other way round.
	live sync.RWMutex

	mu      sync.Mutex
	sio     socketSession
	cancel  context.CancelFunc
	jwt     string
	healthy bool
}

// New builds a client, connects and authenticates.
func New(ctx context.Context, cfg Config) (*Client, error) {
	return newClient(ctx, cfg, false)
}

// NewUnauthenticated connects without logging in, for the handful of events that
// work on an uninitialized instance: needSetup and setup.
func NewUnauthenticated(ctx context.Context, cfg Config) (*Client, error) {
	return newClient(ctx, cfg, true)
}

func newClient(ctx context.Context, cfg Config, skipAuth bool) (*Client, error) {
	c, err := buildClient(ctx, cfg, skipAuth)
	if err != nil {
		return nil, err
	}
	if err := c.session(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

// buildClient assembles a client without dialing. It is separate from newClient
// so the invariants it establishes — a wire.Cache and a base context are always
// present — can be checked without a server to connect to.
func buildClient(ctx context.Context, cfg Config, skipAuth bool) (*Client, error) {
	if cfg.Endpoint == "" {
		return nil, errors.New("endpoint is required")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.MaxRetries < 0 {
		cfg.MaxRetries = 0
	}

	return &Client{
		cfg:   cfg,
		cache: wire.NewCache(),
		// Detached from the caller's context on purpose: the socket has to outlive
		// the Terraform operation that opened it. It must never be nil — dialing
		// derives the connection context from it.
		baseCtx:  context.WithoutCancel(ctx),
		skipAuth: skipAuth,
	}, nil
}

// Close tears down the session.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closeLocked()
}

func (c *Client) closeLocked() error {
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	var err error
	if c.sio != nil {
		err = c.sio.Close()
		c.sio = nil
	}
	c.healthy = false
	return err
}

// markUnhealthy flags the session as unusable so the next call reconnects.
func (c *Client) markUnhealthy() {
	c.mu.Lock()
	c.healthy = false
	c.mu.Unlock()
}

// liveSession returns the current session if it is usable, without dialing.
func (c *Client) liveSession() (socketSession, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sio != nil && c.healthy {
		return c.sio, true
	}
	return nil, false
}

// session makes sure there is a live, authenticated session, reconnecting if
// there is not.
//
// Closing a session cancels the library's client context, and that drops the ack
// callback of every call still waiting on it — silently, since the context branch
// of its per-ack goroutine fires neither callback. So the reconnect takes the
// write lock: it waits for the calls in flight to finish and keeps new ones out
// until the replacement is up. Callers force a reconnect on purpose, to make the
// server resend the push-only lists, and Terraform runs ten resources over this
// one session, so calls in flight during a reconnect are the normal case.
func (c *Client) session(ctx context.Context) error {
	c.live.Lock()
	defer c.live.Unlock()
	return c.sessionLocked(ctx)
}

func (c *Client) sessionLocked(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.sio != nil && c.healthy {
		return nil
	}

	// A stale session is closed before dialing again; the cached lists are
	// dropped because someone else may have changed things while we were gone.
	_ = c.closeLocked()
	c.cache.Invalidate()

	var lastErr error
	attempts := c.cfg.MaxRetries + 1
	for attempt := range attempts {
		if attempt > 0 {
			wait := connectBackoff(attempt, lastErr)
			tflog.Warn(ctx, "uptime kuma: retrying connection", map[string]any{
				"attempt": attempt + 1,
				"wait":    wait.String(),
				"error":   lastErr.Error(),
			})
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(wait):
			}
		}

		if err := c.connectLocked(ctx); err != nil {
			lastErr = err
			_ = c.closeLocked()
			continue
		}
		return nil
	}

	return fmt.Errorf("connecting to %s: %w", c.cfg.Endpoint, lastErr)
}

// http returns the HTTP client used for the few reads that are not available
// over Socket.IO, namely a status page's group tree.
func (c *Client) http() *http.Client {
	c.httpOnce.Do(func() {
		c.httpClient = transport.HTTPClient(&tls.Config{
			InsecureSkipVerify: c.cfg.InsecureSkipVerify, //nolint:gosec // opt-in, for self-signed instances
			MinVersion:         tls.VersionTLS12,
		})
	})
	return c.httpClient
}

// Info returns the last `info` payload pushed by the server.
func (c *Client) Info() wire.ServerInfo {
	return c.cache.GetInfo()
}

// Cache exposes the pushed-list cache, which the api package reads from.
func (c *Client) Cache() *wire.Cache { return c.cache }

// HTTPClient exposes the client used by the one read that goes over HTTP.
func (c *Client) HTTPClient() *http.Client { return c.http() }

// Endpoint returns the configured base URL.
func (c *Client) Endpoint() string { return c.cfg.Endpoint }
