package api

import (
	"context"
	"fmt"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma/wire"
)

// SaveProxy creates or updates a proxy. addProxy is an upsert whose second
// argument is the ID to overwrite, or null to create.
func SaveProxy(ctx context.Context, c Caller, id *int, proxy wire.Proxy) (int, error) {
	var resp struct {
		wire.AckEnvelope
		ID int `json:"id"`
	}
	var idArg any
	if id != nil {
		idArg = *id
	}
	list := c.Cache().Proxies
	var ready func() bool
	if id == nil {
		ready = rowAdded(list, &resp.ID)
	}

	if err := c.MutateUntil(ctx, list, &resp, ready, "addProxy", proxy, idArg); err != nil {
		return 0, err
	}
	if resp.ID == 0 {
		return 0, fmt.Errorf("server accepted the proxy but returned no ID")
	}
	return resp.ID, nil
}

// DeleteProxy removes a proxy and detaches it from the monitors using it.
func DeleteProxy(ctx context.Context, c Caller, id int) error {
	list := c.Cache().Proxies
	return c.MutateUntil(ctx, list, nil, rowGone(list, id), "deleteProxy", id)
}

// ListProxies returns every proxy. Push-only, like notifications.
func ListProxies(ctx context.Context, c Caller) (map[int]wire.Proxy, error) {
	if err := c.EnsureLoaded(ctx, c.Cache().Proxies, nil); err != nil {
		return nil, err
	}
	return c.Cache().Proxies.All(), nil
}

// GetProxy returns one proxy, or wire.ErrNotFound.
func GetProxy(ctx context.Context, c Caller, id int) (*wire.Proxy, error) {
	if err := c.EnsureLoaded(ctx, c.Cache().Proxies, nil); err != nil {
		return nil, err
	}
	proxy, ok := c.Cache().Proxies.Get(id)
	if !ok {
		return nil, wire.ErrNotFound
	}
	return &proxy, nil
}
