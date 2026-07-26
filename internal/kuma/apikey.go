package kuma

import (
	"context"
	"fmt"
)

// CreateAPIKey creates an API key and returns its ID plus the clear-text key.
//
// The clear-text key is shown exactly once, at creation (the server stores only
// a hash), so it can never be recovered — not by a later read and not by import.
//
// These keys authenticate the Prometheus /metrics endpoint, not the Socket.IO
// API this provider speaks. Creating the first one also flips the server's
// `apiKeysEnabled` setting on.
func (c *Client) CreateAPIKey(ctx context.Context, key APIKey) (int, string, error) {
	var resp struct {
		ackEnvelope
		Key   string `json:"key"`
		KeyID int    `json:"keyID"`
	}
	if err := c.mutate(ctx, c.cache.apiKeys, &resp, "addAPIKey", key); err != nil {
		return 0, "", err
	}
	if resp.KeyID == 0 {
		return 0, "", fmt.Errorf("server accepted the API key but returned no ID")
	}
	return resp.KeyID, resp.Key, nil
}

// DeleteAPIKey removes an API key.
func (c *Client) DeleteAPIKey(ctx context.Context, id int) error {
	return c.mutate(ctx, c.cache.apiKeys, nil, "deleteAPIKey", id)
}

// SetAPIKeyActive enables or disables a key without deleting it. There is no
// generic edit event, only the two toggles.
func (c *Client) SetAPIKeyActive(ctx context.Context, id int, active bool) error {
	event := "disableAPIKey"
	if active {
		event = "enableAPIKey"
	}
	return c.mutate(ctx, c.cache.apiKeys, nil, event, id)
}

// ListAPIKeys returns every API key, without the secrets.
func (c *Client) ListAPIKeys(ctx context.Context) (map[int]APIKey, error) {
	refresh := func(ctx context.Context) error {
		return c.refreshList(ctx, c.cache.apiKeys, "getAPIKeyList")
	}
	if err := c.ensureLoaded(ctx, c.cache.apiKeys, refresh); err != nil {
		return nil, err
	}
	return c.cache.apiKeys.all(), nil
}

// GetAPIKey returns one API key, or ErrNotFound.
func (c *Client) GetAPIKey(ctx context.Context, id int) (*APIKey, error) {
	keys, err := c.ListAPIKeys(ctx)
	if err != nil {
		return nil, err
	}
	key, ok := keys[id]
	if !ok {
		return nil, ErrNotFound
	}
	return &key, nil
}
