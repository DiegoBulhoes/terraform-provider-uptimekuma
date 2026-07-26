package kuma

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	engineio "github.com/maldikhan/go.socket.io/engine.io/v4/client"
	engineiopolling "github.com/maldikhan/go.socket.io/engine.io/v4/client/transport/polling"
	engineiows "github.com/maldikhan/go.socket.io/engine.io/v4/client/transport/websocket"
	socketio "github.com/maldikhan/go.socket.io/socket.io/v5/client"
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
	cache *cache

	baseCtx context.Context

	// skipAuth is set by NewUnauthenticated, for talking to an instance that
	// has no user yet.
	skipAuth bool

	httpOnce   sync.Once
	httpClient *http.Client

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
	if _, err := c.session(ctx); err != nil {
		return nil, err
	}
	return c, nil
}

// buildClient assembles a client without dialing. It is separate from newClient
// so the invariants it establishes — a cache and a base context are always
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
		cache: newCache(),
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

// session returns a live, authenticated session, reconnecting if needed.
func (c *Client) session(ctx context.Context) (socketSession, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.sio != nil && c.healthy {
		return c.sio, nil
	}

	// A stale session is closed before dialing again; the cached lists are
	// dropped because someone else may have changed things while we were gone.
	_ = c.closeLocked()
	c.cache.invalidate()

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
				return nil, ctx.Err()
			case <-time.After(wait):
			}
		}

		if err := c.connectLocked(ctx); err != nil {
			lastErr = err
			_ = c.closeLocked()
			continue
		}
		return c.sio, nil
	}

	return nil, fmt.Errorf("connecting to %s: %w", c.cfg.Endpoint, lastErr)
}

// connectBackoff decides how long to wait before retrying a connection.
//
// The login limiter refills 20 tokens per minute, so roughly one every three
// seconds. Ordinary exponential backoff starting at one second burns all its
// attempts inside a single refill window and fails a run that would have
// succeeded, so a rate-limit rejection gets its own, slower schedule.
func connectBackoff(attempt int, err error) time.Duration {
	if IsRateLimited(err) {
		wait := time.Duration(attempt*5) * time.Second
		if wait > 30*time.Second {
			wait = 30 * time.Second
		}
		return wait
	}
	return time.Duration(1<<uint(attempt-1)) * time.Second
}

// connectLocked dials, waits for the Socket.IO handshake and authenticates.
// Callers must hold c.mu.
func (c *Client) connectLocked(ctx context.Context) error {
	endpoint, err := url.Parse(c.cfg.Endpoint)
	if err != nil {
		return fmt.Errorf("invalid endpoint %q: %w", c.cfg.Endpoint, err)
	}
	// Uptime Kuma serves Socket.IO from the default path; anything the user put
	// in the URL path would break the handshake.
	endpoint.Path = "/socket.io/"

	// baseCtx outlives the request that triggered this dial, because the socket
	// has to stay up between Terraform operations. A nil one would panic here and
	// take the whole provider process down, so it falls back rather than trusting
	// the constructor.
	baseCtx := c.baseCtx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	connCtx, cancel := context.WithCancel(baseCtx)
	c.cancel = cancel

	log := &logger{ctx: connCtx}
	tlsConfig := &tls.Config{
		InsecureSkipVerify: c.cfg.InsecureSkipVerify, //nolint:gosec // opt-in, for self-signed instances
		MinVersion:         tls.VersionTLS12,
	}

	wsTransport, err := engineiows.NewTransport(
		engineiows.WithLogger(log),
		engineiows.WithWebSocket(&tlsWebSocket{tlsConfig: tlsConfig}),
	)
	if err != nil {
		return fmt.Errorf("building websocket transport: %w", err)
	}
	pollingTransport, err := engineiopolling.NewTransport(
		engineiopolling.WithLogger(log),
		engineiopolling.WithHTTPClient(httpClient(tlsConfig)),
	)
	if err != nil {
		return fmt.Errorf("building polling transport: %w", err)
	}

	engine, err := engineio.NewClient(
		engineio.WithURL(endpoint),
		engineio.WithLogger(log),
		engineio.WithSupportedTransports([]engineio.Transport{wsTransport, pollingTransport}),
	)
	if err != nil {
		return fmt.Errorf("building engine.io client: %w", err)
	}

	sio, err := socketio.NewClient(
		socketio.WithEngineIOClient(engine),
		socketio.WithLogger(log),
	)
	if err != nil {
		return fmt.Errorf("building socket.io client: %w", err)
	}

	c.registerHandlers(sio)

	// The library has no "wait until connected" primitive, so the connect
	// handler signals through a channel.
	//
	// The handler must take []any. Any other signature goes through the
	// library's reflection path, which requires every argument to be a
	// json.RawMessage — and the connect event carries a nil payload, so the
	// callback would be dropped without a word and this would wait forever.
	connected := make(chan struct{})
	var once sync.Once
	sio.On("connect", func([]any) {
		once.Do(func() { close(connected) })
	})

	if err := sio.Connect(connCtx); err != nil {
		return fmt.Errorf("connecting: %w", err)
	}

	select {
	case <-connected:
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(c.cfg.Timeout):
		return fmt.Errorf("timed out waiting for the Socket.IO handshake at %s", c.cfg.Endpoint)
	}

	c.sio = sio
	c.healthy = true

	if err := c.authenticateLocked(ctx, sio); err != nil {
		c.healthy = false
		return err
	}
	return nil
}

// authenticateLocked logs in, preferring the cached JWT so a reconnect does not
// re-spend the one-time 2FA code.
func (c *Client) authenticateLocked(ctx context.Context, sio socketSession) error {
	if c.skipAuth {
		return nil
	}

	if c.jwt != "" {
		var resp ackEnvelope
		if err := c.callWith(ctx, sio, &resp, "loginByToken", c.jwt); err == nil {
			return nil
		}
		tflog.Debug(ctx, "uptime kuma: cached token rejected, falling back to password login")
		c.jwt = ""
	}

	if c.cfg.Username == "" || c.cfg.Password == "" {
		return errors.New("username and password are required to authenticate")
	}

	var resp struct {
		ackEnvelope
		Token         string `json:"token"`
		TokenRequired bool   `json:"tokenRequired"`
	}
	payload := map[string]any{
		"username": c.cfg.Username,
		"password": c.cfg.Password,
		"token":    c.cfg.TOTPToken,
	}
	if err := c.callWith(ctx, sio, &resp, "login", payload); err != nil {
		return fmt.Errorf("login failed: %w", err)
	}
	if resp.TokenRequired {
		return errors.New("login failed: this account has 2FA enabled, set the provider's `token` attribute to the current code")
	}
	if resp.Token == "" {
		return errors.New("login failed: server returned no session token")
	}
	c.jwt = resp.Token
	return nil
}

// registerHandlers subscribes to the server-pushed lists. This has to happen
// before connecting, because the server starts pushing right after login.
func (c *Client) registerHandlers(sio *socketio.Client) {
	sio.On("disconnect", func([]any) {
		c.markUnhealthy()
	})

	// monitorList and maintenanceList arrive as objects keyed by ID; the rest
	// arrive as arrays.
	sio.On("monitorList", func(in []any) {
		if items, ok := decodeKeyedList[Monitor](in); ok {
			c.cache.monitors.replace(items)
		}
	})
	sio.On("updateMonitorIntoList", func(in []any) {
		if items, ok := decodeKeyedList[Monitor](in); ok {
			c.cache.monitors.patch(items)
		}
	})
	sio.On("deleteMonitorFromList", func(in []any) {
		if id, ok := decodeInt(in); ok {
			c.cache.monitors.remove(id)
		}
	})
	sio.On("maintenanceList", func(in []any) {
		if items, ok := decodeKeyedList[Maintenance](in); ok {
			c.cache.maintenances.replace(items)
		}
	})
	sio.On("notificationList", func(in []any) {
		if items, ok := decodeArrayList(in, func(n Notification) int { return n.ID }); ok {
			c.cache.notifications.replace(items)
		}
	})
	sio.On("proxyList", func(in []any) {
		if items, ok := decodeArrayList(in, func(p Proxy) int { return p.ID }); ok {
			c.cache.proxies.replace(items)
		}
	})
	sio.On("dockerHostList", func(in []any) {
		if items, ok := decodeArrayList(in, func(d DockerHost) int { return d.ID }); ok {
			c.cache.dockerHosts.replace(items)
		}
	})
	sio.On("remoteBrowserList", func(in []any) {
		if items, ok := decodeArrayList(in, func(r RemoteBrowser) int { return r.ID }); ok {
			c.cache.remoteBrowsers.replace(items)
		}
	})
	sio.On("apiKeyList", func(in []any) {
		if items, ok := decodeArrayList(in, func(k APIKey) int { return k.ID }); ok {
			c.cache.apiKeys.replace(items)
		}
	})
	sio.On("statusPageList", func(in []any) {
		if items, ok := decodeKeyedList[StatusPage](in); ok {
			c.cache.statusPages.replace(items)
		}
	})
	sio.On("info", func(in []any) {
		var info ServerInfo
		if raw, ok := firstRaw(in); ok && json.Unmarshal(raw, &info) == nil {
			c.cache.setInfo(info)
		}
	})
}

// http returns the HTTP client used for the few reads that are not available
// over Socket.IO, namely a status page's group tree.
func (c *Client) http() *http.Client {
	c.httpOnce.Do(func() {
		c.httpClient = httpClient(&tls.Config{
			InsecureSkipVerify: c.cfg.InsecureSkipVerify, //nolint:gosec // opt-in, for self-signed instances
			MinVersion:         tls.VersionTLS12,
		})
	})
	return c.httpClient
}

// Info returns the last `info` payload pushed by the server.
func (c *Client) Info() ServerInfo {
	return c.cache.getInfo()
}

func firstRaw(in []any) (json.RawMessage, bool) {
	if len(in) == 0 {
		return nil, false
	}
	raw, ok := in[0].(json.RawMessage)
	return raw, ok
}

// decodeKeyedList decodes an object keyed by stringified ID.
func decodeKeyedList[T any](in []any) (map[int]T, bool) {
	raw, ok := firstRaw(in)
	if !ok {
		return nil, false
	}
	var keyed map[string]T
	if err := json.Unmarshal(raw, &keyed); err != nil {
		return nil, false
	}
	out := make(map[int]T, len(keyed))
	for key, item := range keyed {
		id, err := strconv.Atoi(key)
		if err != nil {
			continue
		}
		out[id] = item
	}
	return out, true
}

// decodeArrayList decodes an array, indexing it by each element's own ID.
func decodeArrayList[T any](in []any, id func(T) int) (map[int]T, bool) {
	raw, ok := firstRaw(in)
	if !ok {
		return nil, false
	}
	var list []T
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, false
	}
	out := make(map[int]T, len(list))
	for _, item := range list {
		out[id(item)] = item
	}
	return out, true
}

func decodeInt(in []any) (int, bool) {
	raw, ok := firstRaw(in)
	if !ok {
		return 0, false
	}
	var v int
	if err := json.Unmarshal(raw, &v); err != nil {
		return 0, false
	}
	return v, true
}
