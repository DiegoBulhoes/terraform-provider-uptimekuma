package api

import (
	"context"
	"fmt"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma/wire"
)

// SaveNotification creates or updates a notification channel.
//
// addNotification is an upsert: the second argument is the ID to overwrite, or
// null to create. The payload is a single flat object — the server stores it
// whole as JSON in notification.config and only lifts `name` and `isDefault`
// into columns, so the provider type and its settings must sit at the top level
// (server/notification.js, wire.Notification.save).
func SaveNotification(ctx context.Context, c Caller, id *int, payload map[string]any) (int, error) {
	var resp struct {
		wire.AckEnvelope
		ID int `json:"id"`
	}

	// nil marshals to null, which is what the server checks for to decide
	// between insert and update.
	var idArg any
	if id != nil {
		idArg = *id
	}

	// A create can wait for its own row to appear; an update cannot, since the
	// row is already in the list whatever the server does with the payload.
	list := c.Cache().Notifications
	var ready func() bool
	if id == nil {
		ready = rowAdded(list, &resp.ID)
	}

	if err := c.MutateUntil(ctx, list, &resp, ready, "addNotification", payload, idArg); err != nil {
		return 0, err
	}
	if resp.ID == 0 {
		return 0, fmt.Errorf("server accepted the notification but returned no ID")
	}
	return resp.ID, nil
}

// DeleteNotification removes a notification channel.
func DeleteNotification(ctx context.Context, c Caller, id int) error {
	list := c.Cache().Notifications
	return c.MutateUntil(ctx, list, nil, rowGone(list, id), "deleteNotification", id)
}

// TestNotification asks the server to deliver a test message.
func TestNotification(ctx context.Context, c Caller, payload map[string]any) error {
	return c.Call(ctx, nil, "testNotification", payload)
}

// ListNotifications returns the notification channels of the authenticated user.
//
// There is no getter event for notifications; the server only pushes the list,
// as part of afterLogin and after every mutation.
func ListNotifications(ctx context.Context, c Caller) (map[int]wire.Notification, error) {
	if err := c.EnsureLoaded(ctx, c.Cache().Notifications, nil); err != nil {
		return nil, err
	}
	return c.Cache().Notifications.All(), nil
}

// GetNotification returns one notification channel, or wire.ErrNotFound.
func GetNotification(ctx context.Context, c Caller, id int) (*wire.Notification, error) {
	if err := c.EnsureLoaded(ctx, c.Cache().Notifications, nil); err != nil {
		return nil, err
	}
	notification, ok := c.Cache().Notifications.Get(id)
	if !ok {
		return nil, wire.ErrNotFound
	}
	return &notification, nil
}
