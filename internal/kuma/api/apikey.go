package api

import (
	"context"
	"fmt"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma/wire"
)

// CreateAPIKey creates an API key and returns its ID plus the clear-text key.
//
// The clear-text key is shown exactly once, at creation (the server stores only
// a hash), so it can never be recovered — not by a later read and not by import.
//
// These keys authenticate the Prometheus /metrics endpoint, not the Socket.IO
// API this provider speaks. Creating the first one also flips the server's
// `apiKeysEnabled` setting on.
func CreateAPIKey(ctx context.Context, c Caller, key wire.APIKey) (int, string, error) {
	var resp struct {
		wire.AckEnvelope
		Key   string `json:"key"`
		KeyID int    `json:"keyID"`
	}
	list := c.Cache().APIKeys
	ready := rowAdded(list, &resp.KeyID)

	if err := c.MutateUntil(ctx, list, &resp, ready, "addAPIKey", key); err != nil {
		return 0, "", err
	}
	if resp.KeyID == 0 {
		return 0, "", fmt.Errorf("server accepted the API key but returned no ID")
	}
	return resp.KeyID, resp.Key, nil
}

// DeleteAPIKey removes an API key.
func DeleteAPIKey(ctx context.Context, c Caller, id int) error {
	list := c.Cache().APIKeys
	return c.MutateUntil(ctx, list, nil, rowGone(list, id), "deleteAPIKey", id)
}

// SetAPIKeyActive enables or disables a key without deleting it. There is no
// generic edit event, only the two toggles.
func SetAPIKeyActive(ctx context.Context, c Caller, id int, active bool) error {
	event := "disableAPIKey"
	if active {
		event = "enableAPIKey"
	}
	return c.Mutate(ctx, c.Cache().APIKeys, nil, event, id)
}

// ListAPIKeys returns every API key, without the secrets.
func ListAPIKeys(ctx context.Context, c Caller) (map[int]wire.APIKey, error) {
	refresh := func(ctx context.Context) error {
		return c.RefreshList(ctx, c.Cache().APIKeys, "getAPIKeyList")
	}
	if err := c.EnsureLoaded(ctx, c.Cache().APIKeys, refresh); err != nil {
		return nil, err
	}
	return c.Cache().APIKeys.All(), nil
}

// GetAPIKey returns one API key, or wire.ErrNotFound.
func GetAPIKey(ctx context.Context, c Caller, id int) (*wire.APIKey, error) {
	keys, err := ListAPIKeys(ctx, c)
	if err != nil {
		return nil, err
	}
	key, ok := keys[id]
	if !ok {
		return nil, wire.ErrNotFound
	}
	return &key, nil
}
