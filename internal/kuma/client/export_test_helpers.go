package client

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/url"
	"time"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma/wire"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma/transport"
)

// Hooks used only by tests.

func ConnectBackoffForTest(attempt int, err error) time.Duration {
	return connectBackoff(attempt, err)
}

func DecodeAckForTest(event string, raw []byte, out any) error {
	return decodeAck(event, raw, out)
}

func (c *Client) InvalidateCachesForTest() {
	c.cache.Invalidate()
}

// NewForHTTPTestOnly builds a complete client that has never connected.
func NewForHTTPTestOnly(endpoint string) *Client {
	return &Client{
		cfg:     Config{Endpoint: endpoint, Timeout: DefaultTimeout},
		cache:   wire.NewCache(),
		baseCtx: context.Background(),
	}
}

// BuildForTest runs the constructor without dialing.
func BuildForTest(ctx context.Context, cfg Config, skipAuth bool) (*Client, error) {
	return buildClient(ctx, cfg, skipAuth)
}

// NewWithoutBaseContextForTest builds the one shape a client must never be in.
func NewWithoutBaseContextForTest(endpoint string) *Client {
	return &Client{
		cfg:   Config{Endpoint: endpoint, Timeout: DefaultTimeout},
		cache: wire.NewCache(),
	}
}

func (c *Client) HasBaseContextForTest() bool { return c.baseCtx != nil }

func (c *Client) TimeoutForTest() time.Duration { return c.cfg.Timeout }

func (c *Client) MaxRetriesForTest() int { return c.cfg.MaxRetries }

// SessionForTest mirrors socketSession, so a test can answer emitted events.
type SessionForTest interface {
	Emit(event any, args ...any) error
	Close() error
}

func (c *Client) InjectSessionForTest(session SessionForTest) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sio = session
	c.healthy = true
	// No cached token: seeding one would send every test down loginByToken.
}

// ForceReconnectForTest drops the session and dials again, the way EnsureLoaded
// does to make the server resend a push-only list.
func (c *Client) ForceReconnectForTest(ctx context.Context) {
	c.markUnhealthy()
	_ = c.session(ctx)
}

func (c *Client) IsHealthyForTest() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.healthy
}

// SeedCachesForTest marks the pushed lists loaded.
func (c *Client) SeedCachesForTest() {
	c.cache.Notifications.Replace(map[int]wire.Notification{})
	c.cache.Proxies.Replace(map[int]wire.Proxy{})
	c.cache.DockerHosts.Replace(map[int]wire.DockerHost{})
	c.cache.RemoteBrowsers.Replace(map[int]wire.RemoteBrowser{})
}

func (c *Client) SetTimeoutForTest(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cfg.Timeout = d
}

type TLSWebSocketForTest = transport.WebSocket

func NewTLSWebSocketForTest(tlsConfig *tls.Config) *TLSWebSocketForTest {
	return transport.NewWebSocket(tlsConfig)
}

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

func HTTPClientForTest(tlsConfig *tls.Config) *http.Client {
	return transport.HTTPClient(tlsConfig)
}

func (c *Client) AuthenticateForTest(ctx context.Context, session SessionForTest) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.authenticateLocked(ctx, session)
}

func (c *Client) SetTokenForTest(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.jwt = token
}

func (c *Client) TokenForTest() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.jwt
}

func (c *Client) SetCredentialsForTest(username, password, totp string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cfg.Username = username
	c.cfg.Password = password
	c.cfg.TOTPToken = totp
}

func (c *Client) SetSkipAuthForTest(skip bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.skipAuth = skip
}

func PoolKeyForTest(cfg Config) string { return poolKey(cfg) }

func SeedPoolForTest(cfg Config, client *Client) {
	poolMu.Lock()
	defer poolMu.Unlock()
	pool[poolKey(cfg)] = client
}

func ResetPoolForTest() {
	poolMu.Lock()
	defer poolMu.Unlock()
	pool = make(map[string]*Client)
}
