package api

import (
	"context"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma/wire"
)

// GetSettings returns the general settings as an untyped map.
//
// The settings table is a key/value store whose contents vary by server version,
// so a typed struct would silently drop keys on upgrade.
func GetSettings(ctx context.Context, c Caller) (map[string]any, error) {
	var resp struct {
		wire.AckEnvelope
		Data map[string]any `json:"data"`
	}
	if err := c.Call(ctx, &resp, "getSettings"); err != nil {
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
func SetSettings(ctx context.Context, c Caller, settings map[string]any, currentPassword string) error {
	return c.Call(ctx, nil, "setSettings", settings, currentPassword)
}

// NeedSetup reports whether the instance has no admin user yet.
//
// This handler answers with a bare boolean rather than the usual `{ok: …}`
// envelope, which is why decodeAck tolerates a non-object acknowledgement.
func NeedSetup(ctx context.Context, c Caller) (bool, error) {
	var need bool
	if err := c.Call(ctx, &need, "needSetup"); err != nil {
		return false, err
	}
	return need, nil
}

// Setup creates the first admin user on a fresh instance. Used by the acceptance
// test bootstrap.
func Setup(ctx context.Context, c Caller, username, password string) error {
	return c.Call(ctx, nil, "setup", username, password)
}
