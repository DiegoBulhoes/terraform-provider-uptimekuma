package client

import (
	"context"
	"fmt"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma/wire"
)

// ensureLoaded makes sure a pushed list has been received at least once.
//
// For entities with a getter, refresh emits it. For the others — notifications,
// proxies, docker hosts and remote browsers — there is no getter at all. The
// server sends those lists exactly once, during afterLogin, so the only way to
// get them again is to log in again: this drops the session so that session()
// reconnects, which makes the server run afterLogin and push everything afresh.
//
// That costs one login against the server's limit of 20 per minute, which is why
// it only happens when the list is genuinely absent. In normal use it never runs:
// the lists arrive with the first login and stay loaded.
func (c *Client) EnsureLoaded(ctx context.Context, list wire.PushedList, refresh func(context.Context) error) error {
	if list.IsLoaded() {
		return nil
	}

	// Subscribe before triggering, so a push that lands immediately is not
	// missed in the gap.
	ch := list.Subscribe()

	if refresh != nil {
		if err := refresh(ctx); err != nil {
			return err
		}
	} else {
		// No getter exists, so force a reconnect and let afterLogin resend.
		c.markUnhealthy()
		if err := c.session(ctx); err != nil {
			return err
		}
	}

	if list.IsLoaded() {
		return nil
	}

	waitCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	wire.WaitUpdate(waitCtx, ch)

	if !list.IsLoaded() {
		return fmt.Errorf("timed out waiting for the server to push the list: %w", wire.ErrTimeout)
	}
	return nil
}

// mutate emits a write event and waits for the list push it triggers.
//
// Every write handler re-sends the affected list before acknowledging, but the
// push travels on a separate channel and its handler runs on another goroutine.
// Without this wait, a Terraform Create followed immediately by a Read could
// observe the pre-mutation wire.Cache. A missed push is not fatal — the next read
// falls back to the cached contents.
func (c *Client) Mutate(ctx context.Context, list wire.PushedList, out any, event string, args ...any) error {
	return c.MutateUntil(ctx, list, out, nil, event, args...)
}

// mutateUntil is mutate with a post-condition.
//
// One push is not enough to see your own write. Terraform applies up to ten
// resources at once over the one shared session, and the pushes they trigger are
// interleaved: a create can be woken by the push another create caused, whose
// list predates its own row. That read then reports the row as missing.
//
// So when a write has a post-condition the caller can state — a create knows the
// ID the acknowledgement returned, a delete knows the ID that must be gone — this
// keeps waiting until the pushed list satisfies it. A nil ready means there is
// none to state, as with an edit of a row that is already there, and one push is
// all that is waited for.
//
// The timeout is not an error, for the same reason it is not in mutate: the
// caller reads the wire.Cache next and reports what it finds there.
func (c *Client) MutateUntil(ctx context.Context, list wire.PushedList, out any, ready func() bool, event string, args ...any) error {
	ch := list.Subscribe()
	if err := c.Call(ctx, out, event, args...); err != nil {
		return err
	}

	waitCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()

	if ready == nil {
		wire.WaitUpdate(waitCtx, ch)
		return nil
	}

	// Subscribe before each check, never after, so a push that lands between the
	// two is still waited on rather than missed.
	for !ready() {
		wire.WaitUpdate(waitCtx, ch)
		if waitCtx.Err() != nil {
			return nil
		}
		ch = list.Subscribe()
	}
	return nil
}

// refreshList emits a getter and waits for the resulting push.
//
// The getters for monitors, maintenances and API keys acknowledge with a bare
// `{ok: true}` and deliver the payload out-of-band on the push channel, so the
// acknowledgement alone does not mean the data has arrived.
func (c *Client) RefreshList(ctx context.Context, list wire.PushedList, event string, args ...any) error {
	ch := list.Subscribe()
	if err := c.Call(ctx, nil, event, args...); err != nil {
		return err
	}
	waitCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	wire.WaitUpdate(waitCtx, ch)
	return nil
}
