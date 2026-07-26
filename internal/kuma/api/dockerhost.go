package api

import (
	"context"
	"fmt"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma/wire"
)

// SaveDockerHost creates or updates a Docker host. Upsert, with the ID as the
// second argument.
func SaveDockerHost(ctx context.Context, c Caller, id *int, host wire.DockerHost) (int, error) {
	var resp struct {
		wire.AckEnvelope
		ID int `json:"id"`
	}
	var idArg any
	if id != nil {
		idArg = *id
	}
	list := c.Cache().DockerHosts
	var ready func() bool
	if id == nil {
		ready = rowAdded(list, &resp.ID)
	}

	if err := c.MutateUntil(ctx, list, &resp, ready, "addDockerHost", host, idArg); err != nil {
		return 0, err
	}
	if resp.ID == 0 {
		return 0, fmt.Errorf("server accepted the docker host but returned no ID")
	}
	return resp.ID, nil
}

// DeleteDockerHost removes a Docker host.
func DeleteDockerHost(ctx context.Context, c Caller, id int) error {
	list := c.Cache().DockerHosts
	return c.MutateUntil(ctx, list, nil, rowGone(list, id), "deleteDockerHost", id)
}

// TestDockerHost asks the server to probe the daemon and returns its message.
func TestDockerHost(ctx context.Context, c Caller, host wire.DockerHost) (string, error) {
	var resp wire.AckEnvelope
	if err := c.Call(ctx, &resp, "testDockerHost", host); err != nil {
		return "", err
	}
	return resp.Msg, nil
}

// ListDockerHosts returns every Docker host. Push-only.
func ListDockerHosts(ctx context.Context, c Caller) (map[int]wire.DockerHost, error) {
	if err := c.EnsureLoaded(ctx, c.Cache().DockerHosts, nil); err != nil {
		return nil, err
	}
	return c.Cache().DockerHosts.All(), nil
}

// GetDockerHost returns one Docker host, or wire.ErrNotFound.
func GetDockerHost(ctx context.Context, c Caller, id int) (*wire.DockerHost, error) {
	if err := c.EnsureLoaded(ctx, c.Cache().DockerHosts, nil); err != nil {
		return nil, err
	}
	host, ok := c.Cache().DockerHosts.Get(id)
	if !ok {
		return nil, wire.ErrNotFound
	}
	return &host, nil
}
