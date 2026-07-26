package client

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma/wire"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/maldikhan/go.socket.io/socket.io/v5/client/emit"
)

// call emits an event and waits for its acknowledgement.
//
// Every API method in this package goes through here. The raw acknowledgement is
// decoded twice: once into the envelope to decide success, and once into out so
// callers get the event-specific fields (`monitorID`, `id`, `key`, `data`…)
// without each of them repeating the plumbing. Pass a nil out to ignore the
// payload.
//
// The read lock is held for the whole emit-and-wait, so a concurrent reconnect
// waits for this call rather than cancelling its acknowledgement. It cannot be
// held across session(), which takes the write lock, hence the two passes: dial
// without it, then take it and check the session is still the live one.
func (c *Client) Call(ctx context.Context, out any, event string, args ...any) error {
	// Two passes at most: the second runs once a session has been established,
	// and only loses the race if another reconnect started in between.
	for range 2 {
		ran, err := c.callOnLiveSession(ctx, out, event, args...)
		if ran {
			return err
		}
		if err := c.session(ctx); err != nil {
			return err
		}
	}
	return fmt.Errorf("emitting %q: %w", event, wire.ErrNotConnected)
}

// callOnLiveSession makes the call under the read lock, if there is a session to
// make it on. ran reports whether it did.
func (c *Client) callOnLiveSession(ctx context.Context, out any, event string, args ...any) (ran bool, err error) {
	c.live.RLock()
	defer c.live.RUnlock()

	sio, ok := c.liveSession()
	if !ok {
		return false, nil
	}
	return true, c.callWith(ctx, sio, out, event, args...)
}

// callWith is call against an explicit session. Connection setup needs it
// because it must emit "login" while already holding the session lock, which
// rules out going through session() again.
func (c *Client) callWith(ctx context.Context, sio socketSession, out any, event string, args ...any) error {
	type ackResult struct {
		raw json.RawMessage
		err error
	}
	// Buffered: the library invokes the callback from its own goroutine, and it
	// must never block if this function has already returned on ctx.
	done := make(chan ackResult, 1)

	emitArgs := make([]any, 0, len(args)+2)
	emitArgs = append(emitArgs, args...)
	emitArgs = append(emitArgs,
		emit.WithAck(func(in []any) {
			res := ackResult{}
			switch len(in) {
			case 0:
				res.err = fmt.Errorf("empty acknowledgement for %q", event)
			default:
				raw, ok := in[0].(json.RawMessage)
				if !ok {
					res.err = fmt.Errorf("unexpected acknowledgement type %T for %q", in[0], event)
					break
				}
				res.raw = raw
			}
			select {
			case done <- res:
			default:
			}
		}),
		emit.WithTimeout(c.cfg.Timeout, func() {
			select {
			case done <- ackResult{err: wire.ErrTimeout}:
			default:
			}
		}),
	)

	tflog.Debug(ctx, "uptime kuma: emitting event", map[string]any{"event": event})

	if err := sio.Emit(event, emitArgs...); err != nil {
		c.markUnhealthy()
		return fmt.Errorf("emitting %q: %w", event, err)
	}

	// The library's own timeout is not enough to bound this wait. Its per-ack
	// goroutine selects on the acknowledgement, the timer and its client context,
	// and the context branch only logs: neither callback fires. Closing the
	// session cancels exactly that context, so a reconnect forced by another
	// goroutine leaves this one waiting on a channel nothing will ever write to.
	// Terraform's apply context carries no deadline, so that wait is forever.
	waitCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()

	select {
	case res := <-done:
		if res.err != nil {
			if res.err == wire.ErrTimeout {
				// A timeout means the session is no longer answering, so drop
				// it and let the next call reconnect.
				c.markUnhealthy()
			}
			return fmt.Errorf("%q: %w", event, res.err)
		}
		return decodeAck(event, res.raw, out)
	case <-waitCtx.Done():
		if ctx.Err() != nil {
			return ctx.Err()
		}
		c.markUnhealthy()
		return fmt.Errorf("%q: %w", event, wire.ErrTimeout)
	}
}

// decodeAck turns a raw acknowledgement into either an error or the caller's
// struct.
func decodeAck(event string, raw json.RawMessage, out any) error {
	var env wire.AckEnvelope
	// A decode failure here is not fatal: handlers that reply with a bare
	// boolean or array produce one, and those responses carry no ok field to
	// begin with.
	if err := json.Unmarshal(raw, &env); err == nil {
		if env.OK != nil && !*env.OK {
			return &wire.APIError{Event: event, Msg: env.Msg}
		}
	}

	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decoding %q response: %w", event, err)
	}
	return nil
}
