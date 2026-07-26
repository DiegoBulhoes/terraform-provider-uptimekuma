// Package transport carries the Socket.IO connection.
//
// It exists because the library's built-in WebSocket hardcodes the default TLS
// config, which rules out the self-signed certificates common on self-hosted
// Uptime Kuma.
package transport

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"net/url"

	"golang.org/x/net/websocket"
)

// WebSocket is the Socket.IO transport's WebSocket, with a custom *tls.Config.
type WebSocket struct {
	tlsConfig *tls.Config

	conn *websocket.Conn
}

var ErrNotConnected = errors.New("websocket connection is not initialized")

func (w *WebSocket) Dial(ctx context.Context, u *url.URL, origin *url.URL) error {
	config, err := websocket.NewConfig(u.String(), origin.String())
	if err != nil {
		return err
	}
	config.TlsConfig = w.tlsConfig

	conn, err := config.DialContext(ctx)
	if err != nil {
		return err
	}
	w.conn = conn
	return nil
}

func (w *WebSocket) Send(v []byte) error {
	if w.conn == nil {
		return ErrNotConnected
	}
	return websocket.Message.Send(w.conn, string(v))
}

func (w *WebSocket) Receive(v *[]byte) error {
	if w.conn == nil {
		return ErrNotConnected
	}
	return websocket.Message.Receive(w.conn, v)
}

func (w *WebSocket) Close() error {
	if w.conn == nil {
		return nil
	}
	return w.conn.Close()
}

// HTTPClient builds the client used by the long-polling transport.
func HTTPClient(tlsConfig *tls.Config) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig
	return &http.Client{Transport: transport}
}

// NewWebSocket returns a WebSocket that has not dialed yet.
func NewWebSocket(tlsConfig *tls.Config) *WebSocket {
	return &WebSocket{tlsConfig: tlsConfig}
}
