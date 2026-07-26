package kuma

import (
	"context"
	"fmt"
)

// SaveDockerHost creates or updates a Docker host. Upsert, with the ID as the
// second argument.
func (c *Client) SaveDockerHost(ctx context.Context, id *int, host DockerHost) (int, error) {
	var resp struct {
		ackEnvelope
		ID int `json:"id"`
	}
	var idArg any
	if id != nil {
		idArg = *id
	}
	if err := c.mutate(ctx, c.cache.dockerHosts, &resp, "addDockerHost", host, idArg); err != nil {
		return 0, err
	}
	if resp.ID == 0 {
		return 0, fmt.Errorf("server accepted the docker host but returned no ID")
	}
	return resp.ID, nil
}

// DeleteDockerHost removes a Docker host.
func (c *Client) DeleteDockerHost(ctx context.Context, id int) error {
	return c.mutate(ctx, c.cache.dockerHosts, nil, "deleteDockerHost", id)
}

// TestDockerHost asks the server to probe the daemon and returns its message.
func (c *Client) TestDockerHost(ctx context.Context, host DockerHost) (string, error) {
	var resp ackEnvelope
	if err := c.call(ctx, &resp, "testDockerHost", host); err != nil {
		return "", err
	}
	return resp.Msg, nil
}

// ListDockerHosts returns every Docker host. Push-only.
func (c *Client) ListDockerHosts(ctx context.Context) (map[int]DockerHost, error) {
	if err := c.ensureLoaded(ctx, c.cache.dockerHosts, nil); err != nil {
		return nil, err
	}
	return c.cache.dockerHosts.all(), nil
}

// GetDockerHost returns one Docker host, or ErrNotFound.
func (c *Client) GetDockerHost(ctx context.Context, id int) (*DockerHost, error) {
	if err := c.ensureLoaded(ctx, c.cache.dockerHosts, nil); err != nil {
		return nil, err
	}
	host, ok := c.cache.dockerHosts.get(id)
	if !ok {
		return nil, ErrNotFound
	}
	return &host, nil
}
