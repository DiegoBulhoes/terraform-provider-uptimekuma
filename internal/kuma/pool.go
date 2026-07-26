package kuma

import (
	"context"
	"fmt"
	"sync"
)

// Uptime Kuma limits logins to 20 per minute for the entire server, and every
// client construction spends one. Terraform configures a provider instance per
// command — and the acceptance-test framework does so several times per step —
// so identical configurations sharing one authenticated session is the
// difference between working and being rejected.
//
// Sharing is safe because Client is goroutine-safe and reconnects on demand: a
// session handed out here is either live or will re-establish itself on the next
// call.
var (
	poolMu sync.Mutex
	pool   = make(map[string]*Client)
)

// poolKey identifies configurations that may share a session.
//
// The password is part of the key so that rotating it forces a fresh login
// instead of silently reusing a session opened with the old credentials.
// insecure_skip_verify is too: a caller that stopped accepting self-signed
// certificates must not be handed a session established while it did.
func poolKey(cfg Config) string {
	return fmt.Sprintf("%s|%s|%s|%t", cfg.Endpoint, cfg.Username, cfg.Password, cfg.InsecureSkipVerify)
}

// Shared returns a client for the given configuration, reusing an existing
// session when one was already opened for the same endpoint and user in this
// process.
func Shared(ctx context.Context, cfg Config) (*Client, error) {
	key := poolKey(cfg)

	poolMu.Lock()
	defer poolMu.Unlock()

	if client, ok := pool[key]; ok {
		return client, nil
	}

	client, err := New(ctx, cfg)
	if err != nil {
		return nil, err
	}
	pool[key] = client
	return client, nil
}
