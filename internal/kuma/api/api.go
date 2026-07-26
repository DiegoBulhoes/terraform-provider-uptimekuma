// Package api holds one function per Uptime Kuma operation, grouped by entity.
//
// The functions take a Caller rather than hanging off the client, which is what
// lets them live in their own package. kuma.Client implements Caller and keeps a
// method per operation, so callers still write client.CreateMonitor(...).
package api

import (
	"context"
	"net/http"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma/wire"
)

// Caller is the slice of the client these functions need.
type Caller interface {
	// Call emits an event and decodes its acknowledgement into out.
	Call(ctx context.Context, out any, event string, args ...any) error
	// Mutate emits a write and waits for the list push it triggers, for the
	// entities the server has no getter for.
	Mutate(ctx context.Context, list wire.PushedList, out any, event string, args ...any) error
	// EnsureLoaded makes sure a pushed list has arrived, refreshing if it has not.
	EnsureLoaded(ctx context.Context, list wire.PushedList, refresh func(context.Context) error) error
	// RefreshList asks the server to resend a list.
	RefreshList(ctx context.Context, list wire.PushedList, event string, args ...any) error
	// Cache is the pushed-list cache.
	Cache() *wire.Cache
	// HTTPClient serves the one read that does not go over Socket.IO.
	HTTPClient() *http.Client
	// Endpoint is the base URL, needed to build that HTTP request.
	Endpoint() string
}
