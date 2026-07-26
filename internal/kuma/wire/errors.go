package wire

import (
	"errors"
	"fmt"
	"strings"
)

// ErrNotFound signals that the requested entity does not exist on the server.
// Resources use it in Read to drop the object from state instead of failing.
var ErrNotFound = errors.New("not found")

// ErrTimeout signals that the server accepted the event but never sent the
// acknowledgement within the configured timeout.
var ErrTimeout = errors.New("timeout waiting for server acknowledgement")

// ErrNotConnected signals that the client has no usable Socket.IO session.
var ErrNotConnected = errors.New("not connected to Uptime Kuma")

// ErrRateLimited signals that Uptime Kuma's rate limiter rejected the call.
//
// The login limiter allows 20 attempts per minute for the whole server, and
// every provider instance logs in once — so a plan followed by an apply, or a
// pipeline running several workspaces, can reach it. Waiting is the only fix,
// which is why this counts as retryable.
var ErrRateLimited = errors.New("rate limited by Uptime Kuma")

// APIError is returned when Uptime Kuma answers an event with `ok: false`.
// The server does not use error codes, only free-form (and often i18n-key)
// messages, so callers must match on the message text.
type APIError struct {
	// Event is the Socket.IO event that failed, e.g. "editMonitor".
	Event string
	// Msg is the message the server put in the acknowledgement.
	Msg string
}

func (e *APIError) Error() string {
	if e.Msg == "" {
		return fmt.Sprintf("uptime kuma rejected %q", e.Event)
	}
	return fmt.Sprintf("uptime kuma rejected %q: %s", e.Event, e.Msg)
}

// Is maps server messages that mean "the entity is gone" onto ErrNotFound.
//
// Uptime Kuma has no distinct not-found response: a getter for a missing row
// dereferences a null bean, so the server reports a JavaScript TypeError such
// as "Cannot read properties of null (reading 'id')". Those messages are the
// only signal available.
func (e *APIError) Is(target error) bool {
	msg := strings.ToLower(e.Msg)

	if target == ErrRateLimited {
		// The limiter answers with a fixed message and no code.
		return strings.Contains(msg, "too frequently")
	}

	if target != ErrNotFound {
		return false
	}
	for _, needle := range []string{
		"not found",
		// i18n keys such as "tagNotFound" arrive unspaced.
		"notfound",
		// Status page handlers throw "No slug?" for an unknown slug, and
		// "slug is not found" from the incident handlers.
		"no slug",
		"of null",
		"of undefined",
		"cannot read propert",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

// IsNotFound reports whether an error means the entity is gone, so a Read can
// remove it from state instead of failing the run.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// IsRateLimited reports whether the server's rate limiter rejected the call.
func IsRateLimited(err error) bool {
	return errors.Is(err, ErrRateLimited)
}

// IsRetryable reports whether an error is transient and the call is worth
// repeating. Connection loss and acknowledgement timeouts qualify; a server
// rejection does not, because replaying it would fail the same way.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrNotFound) {
		return false
	}
	if errors.Is(err, ErrTimeout) || errors.Is(err, ErrNotConnected) || errors.Is(err, ErrRateLimited) {
		return true
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return false
	}
	// Anything left is a transport or encoding failure from the Socket.IO
	// layer, which a reconnect may fix.
	return true
}
