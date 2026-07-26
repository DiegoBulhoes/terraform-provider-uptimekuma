package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma/transport"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma/wire"

	engineio "github.com/maldikhan/go.socket.io/engine.io/v4/client"
	engineiopolling "github.com/maldikhan/go.socket.io/engine.io/v4/client/transport/polling"
	engineiows "github.com/maldikhan/go.socket.io/engine.io/v4/client/transport/websocket"
	socketio "github.com/maldikhan/go.socket.io/socket.io/v5/client"
)

// Dialing: the retry schedule, the transport stack, and the handshake wait.

// connectBackoff decides how long to wait before retrying a connection.
//
// The login limiter refills 20 tokens per minute, so roughly one every three
// seconds. Ordinary exponential backoff starting at one second burns all its
// attempts inside a single refill window and fails a run that would have
// succeeded, so a rate-limit rejection gets its own, slower schedule.
func connectBackoff(attempt int, err error) time.Duration {
	if wire.IsRateLimited(err) {
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
	connCtx := c.newConnectionContext()

	sio, err := c.buildSocket(connCtx)
	if err != nil {
		return err
	}

	c.registerHandlers(sio)

	if err := c.awaitHandshake(ctx, connCtx, sio); err != nil {
		return err
	}

	c.sio = sio
	c.healthy = true

	if err := c.authenticateLocked(ctx, sio); err != nil {
		c.healthy = false
		return err
	}
	return nil
}

// newConnectionContext derives the context the socket lives in and stores its
// cancel func.
//
// It comes from baseCtx rather than the caller's context because the socket has
// to stay up between Terraform operations. A nil baseCtx would panic, so it falls
// back instead of trusting the constructor.
func (c *Client) newConnectionContext() context.Context {
	baseCtx := c.baseCtx
	if baseCtx == nil {
		baseCtx = context.Background()
	}
	connCtx, cancel := context.WithCancel(baseCtx)
	c.cancel = cancel
	return connCtx
}

// socketURL is the endpoint with the path Uptime Kuma serves Socket.IO from.
// Anything the user put in the path would break the handshake.
func (c *Client) socketURL() (*url.URL, error) {
	endpoint, err := url.Parse(c.cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint %q: %w", c.cfg.Endpoint, err)
	}
	endpoint.Path = "/socket.io/"
	return endpoint, nil
}

// buildSocket assembles the transport stack: a WebSocket and a long-polling
// transport, an Engine.IO client over both, and Socket.IO on top.
func (c *Client) buildSocket(connCtx context.Context) (*socketio.Client, error) {
	endpoint, err := c.socketURL()
	if err != nil {
		return nil, err
	}

	log := transport.NewLogger(connCtx)
	tlsConfig := &tls.Config{
		InsecureSkipVerify: c.cfg.InsecureSkipVerify, //nolint:gosec // opt-in, for self-signed instances
		MinVersion:         tls.VersionTLS12,
	}

	wsTransport, err := engineiows.NewTransport(
		engineiows.WithLogger(log),
		engineiows.WithWebSocket(transport.NewWebSocket(tlsConfig)),
	)
	if err != nil {
		return nil, fmt.Errorf("building websocket transport: %w", err)
	}
	pollingTransport, err := engineiopolling.NewTransport(
		engineiopolling.WithLogger(log),
		engineiopolling.WithHTTPClient(transport.HTTPClient(tlsConfig)),
	)
	if err != nil {
		return nil, fmt.Errorf("building polling transport: %w", err)
	}

	engine, err := engineio.NewClient(
		engineio.WithURL(endpoint),
		engineio.WithLogger(log),
		engineio.WithSupportedTransports([]engineio.Transport{wsTransport, pollingTransport}),
	)
	if err != nil {
		return nil, fmt.Errorf("building engine.io client: %w", err)
	}

	sio, err := socketio.NewClient(
		socketio.WithEngineIOClient(engine),
		socketio.WithLogger(log),
	)
	if err != nil {
		return nil, fmt.Errorf("building socket.io client: %w", err)
	}
	return sio, nil
}

// awaitHandshake connects and blocks until the server answers.
//
// The library has no "wait until connected" primitive, so the connect handler
// signals through a channel. That handler must take []any: any other signature
// takes the library's reflection path, which requires every argument to be a
// json.RawMessage, and the connect event carries a nil payload — the callback
// would be dropped without a word and this would wait forever.
func (c *Client) awaitHandshake(ctx, connCtx context.Context, sio *socketio.Client) error {
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
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(c.cfg.Timeout):
		return fmt.Errorf("timed out waiting for the Socket.IO handshake at %s", c.cfg.Endpoint)
	}
}
