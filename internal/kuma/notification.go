package kuma

import (
	"context"
	"fmt"
)

// SaveNotification creates or updates a notification channel.
//
// addNotification is an upsert: the second argument is the ID to overwrite, or
// null to create. The payload is a single flat object — the server stores it
// whole as JSON in notification.config and only lifts `name` and `isDefault`
// into columns, so the provider type and its settings must sit at the top level
// (server/notification.js, Notification.save).
func (c *Client) SaveNotification(ctx context.Context, id *int, payload map[string]any) (int, error) {
	var resp struct {
		ackEnvelope
		ID int `json:"id"`
	}

	// nil marshals to null, which is what the server checks for to decide
	// between insert and update.
	var idArg any
	if id != nil {
		idArg = *id
	}

	if err := c.mutate(ctx, c.cache.notifications, &resp, "addNotification", payload, idArg); err != nil {
		return 0, err
	}
	if resp.ID == 0 {
		return 0, fmt.Errorf("server accepted the notification but returned no ID")
	}
	return resp.ID, nil
}

// DeleteNotification removes a notification channel.
func (c *Client) DeleteNotification(ctx context.Context, id int) error {
	return c.mutate(ctx, c.cache.notifications, nil, "deleteNotification", id)
}

// TestNotification asks the server to deliver a test message.
func (c *Client) TestNotification(ctx context.Context, payload map[string]any) error {
	return c.call(ctx, nil, "testNotification", payload)
}

// ListNotifications returns the notification channels of the authenticated user.
//
// There is no getter event for notifications; the server only pushes the list,
// as part of afterLogin and after every mutation.
func (c *Client) ListNotifications(ctx context.Context) (map[int]Notification, error) {
	if err := c.ensureLoaded(ctx, c.cache.notifications, nil); err != nil {
		return nil, err
	}
	return c.cache.notifications.all(), nil
}

// GetNotification returns one notification channel, or ErrNotFound.
func (c *Client) GetNotification(ctx context.Context, id int) (*Notification, error) {
	if err := c.ensureLoaded(ctx, c.cache.notifications, nil); err != nil {
		return nil, err
	}
	notification, ok := c.cache.notifications.get(id)
	if !ok {
		return nil, ErrNotFound
	}
	return &notification, nil
}
