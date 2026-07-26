package kuma

import (
	"context"
	"fmt"
)

// SaveRemoteBrowser creates or updates a remote browser. Upsert, with the ID as
// the second argument.
func (c *Client) SaveRemoteBrowser(ctx context.Context, id *int, browser RemoteBrowser) (int, error) {
	var resp struct {
		ackEnvelope
		ID int `json:"id"`
	}
	var idArg any
	if id != nil {
		idArg = *id
	}
	if err := c.mutate(ctx, c.cache.remoteBrowsers, &resp, "addRemoteBrowser", browser, idArg); err != nil {
		return 0, err
	}
	if resp.ID == 0 {
		return 0, fmt.Errorf("server accepted the remote browser but returned no ID")
	}
	return resp.ID, nil
}

// DeleteRemoteBrowser removes a remote browser.
func (c *Client) DeleteRemoteBrowser(ctx context.Context, id int) error {
	return c.mutate(ctx, c.cache.remoteBrowsers, nil, "deleteRemoteBrowser", id)
}

// ListRemoteBrowsers returns every remote browser. Push-only.
func (c *Client) ListRemoteBrowsers(ctx context.Context) (map[int]RemoteBrowser, error) {
	if err := c.ensureLoaded(ctx, c.cache.remoteBrowsers, nil); err != nil {
		return nil, err
	}
	return c.cache.remoteBrowsers.all(), nil
}

// GetRemoteBrowser returns one remote browser, or ErrNotFound.
func (c *Client) GetRemoteBrowser(ctx context.Context, id int) (*RemoteBrowser, error) {
	if err := c.ensureLoaded(ctx, c.cache.remoteBrowsers, nil); err != nil {
		return nil, err
	}
	browser, ok := c.cache.remoteBrowsers.get(id)
	if !ok {
		return nil, ErrNotFound
	}
	return &browser, nil
}
