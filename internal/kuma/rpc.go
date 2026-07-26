package kuma

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/maldikhan/go.socket.io/socket.io/v5/client/emit"
)

// ackEnvelope is the shape shared by nearly every Uptime Kuma acknowledgement.
//
// OK is a pointer because a handful of handlers answer with a bare value
// instead of an object — `needSetup` replies with a boolean — and those must not
// be treated as failures.
type ackEnvelope struct {
	OK  *bool  `json:"ok"`
	Msg string `json:"msg"`
}

// call emits an event and waits for its acknowledgement.
//
// Every API method in this package goes through here. The raw acknowledgement is
// decoded twice: once into the envelope to decide success, and once into out so
// callers get the event-specific fields (`monitorID`, `id`, `key`, `data`…)
// without each of them repeating the plumbing. Pass a nil out to ignore the
// payload.
func (c *Client) call(ctx context.Context, out any, event string, args ...any) error {
	sio, err := c.session(ctx)
	if err != nil {
		return err
	}
	return c.callWith(ctx, sio, out, event, args...)
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
			case done <- ackResult{err: ErrTimeout}:
			default:
			}
		}),
	)

	tflog.Debug(ctx, "uptime kuma: emitting event", map[string]any{"event": event})

	if err := sio.Emit(event, emitArgs...); err != nil {
		c.markUnhealthy()
		return fmt.Errorf("emitting %q: %w", event, err)
	}

	select {
	case res := <-done:
		if res.err != nil {
			if res.err == ErrTimeout {
				// A timeout means the session is no longer answering, so drop
				// it and let the next call reconnect.
				c.markUnhealthy()
			}
			return fmt.Errorf("%q: %w", event, res.err)
		}
		return decodeAck(event, res.raw, out)
	case <-ctx.Done():
		return ctx.Err()
	}
}

// decodeAck turns a raw acknowledgement into either an error or the caller's
// struct.
func decodeAck(event string, raw json.RawMessage, out any) error {
	var env ackEnvelope
	// A decode failure here is not fatal: handlers that reply with a bare
	// boolean or array produce one, and those responses carry no ok field to
	// begin with.
	if err := json.Unmarshal(raw, &env); err == nil {
		if env.OK != nil && !*env.OK {
			return &APIError{Event: event, Msg: env.Msg}
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
