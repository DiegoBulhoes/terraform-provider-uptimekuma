package kuma

import (
	"context"
	"fmt"
)

// pushedList is the read side of a server-pushed list, shared by every entity
// whose state only arrives through push events.
type pushedList interface {
	isLoaded() bool
	subscribe() <-chan struct{}
}

// ensureLoaded makes sure a pushed list has been received at least once.
//
// For entities with a getter, refresh emits it. For the others — notifications,
// proxies, docker hosts, remote browsers — there is no getter at all: the server
// only pushes those lists as part of afterLogin, so the best available action is
// to make sure a session exists and then wait for the push.
func (c *Client) ensureLoaded(ctx context.Context, list pushedList, refresh func(context.Context) error) error {
	if list.isLoaded() {
		return nil
	}

	// Subscribe before triggering, so a push that lands immediately is not
	// missed in the gap.
	ch := list.subscribe()

	if refresh != nil {
		if err := refresh(ctx); err != nil {
			return err
		}
	} else if _, err := c.session(ctx); err != nil {
		return err
	}

	if list.isLoaded() {
		return nil
	}

	waitCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	waitUpdate(waitCtx, ch)

	if !list.isLoaded() {
		return fmt.Errorf("timed out waiting for the server to push the list: %w", ErrTimeout)
	}
	return nil
}

// mutate emits a write event and waits for the list push it triggers.
//
// Every write handler re-sends the affected list before acknowledging, but the
// push travels on a separate channel and its handler runs on another goroutine.
// Without this wait, a Terraform Create followed immediately by a Read could
// observe the pre-mutation cache. A missed push is not fatal — the next read
// falls back to the cached contents.
func (c *Client) mutate(ctx context.Context, list pushedList, out any, event string, args ...any) error {
	ch := list.subscribe()
	if err := c.call(ctx, out, event, args...); err != nil {
		return err
	}
	waitCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	waitUpdate(waitCtx, ch)
	return nil
}

// refreshList emits a getter and waits for the resulting push.
//
// The getters for monitors, maintenances and API keys acknowledge with a bare
// `{ok: true}` and deliver the payload out-of-band on the push channel, so the
// acknowledgement alone does not mean the data has arrived.
func (c *Client) refreshList(ctx context.Context, list pushedList, event string, args ...any) error {
	ch := list.subscribe()
	if err := c.call(ctx, nil, event, args...); err != nil {
		return err
	}
	waitCtx, cancel := context.WithTimeout(ctx, c.cfg.Timeout)
	defer cancel()
	waitUpdate(waitCtx, ch)
	return nil
}
