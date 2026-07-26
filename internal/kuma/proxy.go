package kuma

import (
	"context"
	"fmt"
)

// SaveProxy creates or updates a proxy. addProxy is an upsert whose second
// argument is the ID to overwrite, or null to create.
func (c *Client) SaveProxy(ctx context.Context, id *int, proxy Proxy) (int, error) {
	var resp struct {
		ackEnvelope
		ID int `json:"id"`
	}
	var idArg any
	if id != nil {
		idArg = *id
	}
	if err := c.mutate(ctx, c.cache.proxies, &resp, "addProxy", proxy, idArg); err != nil {
		return 0, err
	}
	if resp.ID == 0 {
		return 0, fmt.Errorf("server accepted the proxy but returned no ID")
	}
	return resp.ID, nil
}

// DeleteProxy removes a proxy and detaches it from the monitors using it.
func (c *Client) DeleteProxy(ctx context.Context, id int) error {
	return c.mutate(ctx, c.cache.proxies, nil, "deleteProxy", id)
}

// ListProxies returns every proxy. Push-only, like notifications.
func (c *Client) ListProxies(ctx context.Context) (map[int]Proxy, error) {
	if err := c.ensureLoaded(ctx, c.cache.proxies, nil); err != nil {
		return nil, err
	}
	return c.cache.proxies.all(), nil
}

// GetProxy returns one proxy, or ErrNotFound.
func (c *Client) GetProxy(ctx context.Context, id int) (*Proxy, error) {
	if err := c.ensureLoaded(ctx, c.cache.proxies, nil); err != nil {
		return nil, err
	}
	proxy, ok := c.cache.proxies.get(id)
	if !ok {
		return nil, ErrNotFound
	}
	return &proxy, nil
}
