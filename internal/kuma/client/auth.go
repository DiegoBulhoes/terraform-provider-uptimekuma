package client

import (
	"context"
	"errors"
	"fmt"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma/wire"

	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Login. A cached JWT is preferred so a reconnect neither spends one of the
// server's 20 logins per minute nor burns a single-use 2FA code.

// authenticateLocked logs in, preferring the cached JWT so a reconnect does not
// re-spend the one-time 2FA code.
func (c *Client) authenticateLocked(ctx context.Context, sio socketSession) error {
	if c.skipAuth {
		return nil
	}

	if c.jwt != "" {
		var resp wire.AckEnvelope
		if err := c.callWith(ctx, sio, &resp, "loginByToken", c.jwt); err == nil {
			return nil
		}
		tflog.Debug(ctx, "uptime kuma: cached token rejected, falling back to password login")
		c.jwt = ""
	}

	if c.cfg.Username == "" || c.cfg.Password == "" {
		return errors.New("username and password are required to authenticate")
	}

	var resp struct {
		wire.AckEnvelope
		Token         string `json:"token"`
		TokenRequired bool   `json:"tokenRequired"`
	}
	payload := map[string]any{
		"username": c.cfg.Username,
		"password": c.cfg.Password,
		"token":    c.cfg.TOTPToken,
	}
	if err := c.callWith(ctx, sio, &resp, "login", payload); err != nil {
		return fmt.Errorf("login failed: %w", err)
	}
	if resp.TokenRequired {
		return errors.New("login failed: this account has 2FA enabled, set the provider's `token` attribute to the current code")
	}
	if resp.Token == "" {
		return errors.New("login failed: server returned no session token")
	}
	c.jwt = resp.Token
	return nil
}
