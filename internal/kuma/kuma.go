// Package kuma is the entry point to the Uptime Kuma client.
//
// The implementation is split by responsibility:
//
//   - client — the session: dialing, login, the RPC primitive, the pushed-list cache
//   - api — one function per operation, grouped by entity
//   - wire — the payloads the server sends and expects, and their decoding
//   - transport — the WebSocket, which needs a custom TLS config
//
// This package re-exports what a caller needs, so one import is enough and the
// split stays an implementation detail.
package kuma

import (
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma/api"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma/client"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma/wire"
)

// The client.
type (
	Client = client.Client
	Config = client.Config
)

// DefaultTimeout is the per-call acknowledgement timeout when none is set.
const DefaultTimeout = client.DefaultTimeout

var (
	// New builds a client, connects and authenticates.
	New = client.New

	// NewUnauthenticated connects without logging in, for the events that work on
	// an instance with no user yet: needSetup and setup.
	NewUnauthenticated = client.NewUnauthenticated

	// Shared reuses one authenticated session per configuration in this process,
	// because the server allows only 20 logins per minute.
	Shared = client.Shared
)

// The wire format.
type (
	Monitor             = wire.Monitor
	MonitorTag          = wire.MonitorTag
	Tag                 = wire.Tag
	Notification        = wire.Notification
	NotificationRequest = wire.NotificationRequest
	Maintenance         = wire.Maintenance
	TimePart            = wire.TimePart
	Proxy               = wire.Proxy
	DockerHost          = wire.DockerHost
	RemoteBrowser       = wire.RemoteBrowser
	APIKey              = wire.APIKey
	ServerInfo          = wire.ServerInfo
	StatusPage          = wire.StatusPage
	StatusPageGroup     = wire.StatusPageGroup
	StatusPageMonitor   = wire.StatusPageMonitor
	StatusPageIncident  = wire.StatusPageIncident

	// Bool absorbs the 0/1 booleans SQLite hands back through JSON.
	Bool = wire.Bool

	// APIError is a rejection the server acknowledged with ok:false.
	APIError = wire.APIError
)

// Sentinel errors, matched with errors.Is.
var (
	ErrNotFound     = wire.ErrNotFound
	ErrTimeout      = wire.ErrTimeout
	ErrRateLimited  = wire.ErrRateLimited
	ErrNotConnected = wire.ErrNotConnected
)

// MinIntervalSeconds is the smallest check interval the server accepts.
const MinIntervalSeconds = api.MinIntervalSeconds

// BoolPtr returns a pointer to a wire boolean, for the fields where "unset" and
// "false" mean different things.
func BoolPtr(v bool) *Bool { return wire.BoolPtr(v) }

// IsNotFound reports whether the error means the row is gone.
//
// Uptime Kuma has no distinct not-found response: a missing row makes the server
// dereference null and report a JavaScript TypeError, so this matches on the
// message text.
func IsNotFound(err error) bool { return wire.IsNotFound(err) }

// IsRateLimited reports whether the server refused because of its login limiter.
func IsRateLimited(err error) bool { return wire.IsRateLimited(err) }

// IsRetryable reports whether retrying the call could succeed.
func IsRetryable(err error) bool { return wire.IsRetryable(err) }

// NormalizeMonitor fills in the fields the server dereferences without checking.
func NormalizeMonitor(m *Monitor) { api.NormalizeMonitor(m) }

// NormalizeMaintenance does the same for a maintenance window.
func NormalizeMaintenance(m *Maintenance) { api.NormalizeMaintenance(m) }
