package kuma

import "context"

// GetSettings returns the general settings as an untyped map.
//
// The settings table is a key/value store whose contents vary by server version,
// so a typed struct would silently drop keys on upgrade.
func (c *Client) GetSettings(ctx context.Context) (map[string]any, error) {
	var resp struct {
		ackEnvelope
		Data map[string]any `json:"data"`
	}
	if err := c.call(ctx, &resp, "getSettings"); err != nil {
		return nil, err
	}
	if resp.Data == nil {
		resp.Data = map[string]any{}
	}
	return resp.Data, nil
}

// SetSettings writes general settings.
//
// currentPassword is only checked when the call would disable authentication.
// The provider deliberately does not expose `disableAuth`: turning it on makes
// the server disconnect every client, including this one (server/server.js).
func (c *Client) SetSettings(ctx context.Context, settings map[string]any, currentPassword string) error {
	return c.call(ctx, nil, "setSettings", settings, currentPassword)
}

// NeedSetup reports whether the instance has no admin user yet.
//
// This handler answers with a bare boolean rather than the usual `{ok: …}`
// envelope, which is why decodeAck tolerates a non-object acknowledgement.
func (c *Client) NeedSetup(ctx context.Context) (bool, error) {
	var need bool
	if err := c.call(ctx, &need, "needSetup"); err != nil {
		return false, err
	}
	return need, nil
}

// Setup creates the first admin user on a fresh instance. Used by the acceptance
// test bootstrap.
func (c *Client) Setup(ctx context.Context, username, password string) error {
	return c.call(ctx, nil, "setup", username, password)
}
