package kuma

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"net/url"

	"golang.org/x/net/websocket"
)

// tlsWebSocket implements the WebSocket interface of the Socket.IO transport so
// a custom *tls.Config can be applied. The library's built-in implementation
// hardcodes the default config, which makes self-signed certificates — common
// on self-hosted Uptime Kuma instances — unusable.
type tlsWebSocket struct {
	tlsConfig *tls.Config

	conn *websocket.Conn
}

var errWSNotConnected = errors.New("websocket connection is not initialized")

func (w *tlsWebSocket) Dial(ctx context.Context, u *url.URL, origin *url.URL) error {
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

func (w *tlsWebSocket) Send(v []byte) error {
	if w.conn == nil {
		return errWSNotConnected
	}
	return websocket.Message.Send(w.conn, string(v))
}

func (w *tlsWebSocket) Receive(v *[]byte) error {
	if w.conn == nil {
		return errWSNotConnected
	}
	return websocket.Message.Receive(w.conn, v)
}

func (w *tlsWebSocket) Close() error {
	if w.conn == nil {
		return nil
	}
	return w.conn.Close()
}

// httpClient builds the HTTP client used by the long-polling transport, which
// carries the Engine.IO handshake before the upgrade to WebSocket.
func httpClient(tlsConfig *tls.Config) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig
	return &http.Client{Transport: transport}
}
