package api

import (
	"context"
	"fmt"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma/wire"
)

// SaveRemoteBrowser creates or updates a remote browser. Upsert, with the ID as
// the second argument.
func SaveRemoteBrowser(ctx context.Context, c Caller, id *int, browser wire.RemoteBrowser) (int, error) {
	var resp struct {
		wire.AckEnvelope
		ID int `json:"id"`
	}
	var idArg any
	if id != nil {
		idArg = *id
	}
	if err := c.Mutate(ctx, c.Cache().RemoteBrowsers, &resp, "addRemoteBrowser", browser, idArg); err != nil {
		return 0, err
	}
	if resp.ID == 0 {
		return 0, fmt.Errorf("server accepted the remote browser but returned no ID")
	}
	return resp.ID, nil
}

// DeleteRemoteBrowser removes a remote browser.
func DeleteRemoteBrowser(ctx context.Context, c Caller, id int) error {
	return c.Mutate(ctx, c.Cache().RemoteBrowsers, nil, "deleteRemoteBrowser", id)
}

// ListRemoteBrowsers returns every remote browser. Push-only.
func ListRemoteBrowsers(ctx context.Context, c Caller) (map[int]wire.RemoteBrowser, error) {
	if err := c.EnsureLoaded(ctx, c.Cache().RemoteBrowsers, nil); err != nil {
		return nil, err
	}
	return c.Cache().RemoteBrowsers.All(), nil
}

// GetRemoteBrowser returns one remote browser, or wire.ErrNotFound.
func GetRemoteBrowser(ctx context.Context, c Caller, id int) (*wire.RemoteBrowser, error) {
	if err := c.EnsureLoaded(ctx, c.Cache().RemoteBrowsers, nil); err != nil {
		return nil, err
	}
	browser, ok := c.Cache().RemoteBrowsers.Get(id)
	if !ok {
		return nil, wire.ErrNotFound
	}
	return &browser, nil
}
