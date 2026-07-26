// Command kuma-bootstrap creates the first admin user on an Uptime Kuma
// instance.
//
// A brand-new instance has no account at all, and the only way to create one is
// the `setup` Socket.IO event — there is no HTTP endpoint for it. This is what
// the demo environment and the acceptance tests use to get from a fresh
// container to something a provider can log into.
//
// Running it against an instance that already has an account does nothing.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
)

func main() {
	endpoint := flag.String("endpoint", envOr("UPTIME_KUMA_URL", "http://localhost:3001"), "Uptime Kuma base URL")
	username := flag.String("username", envOr("UPTIME_KUMA_USERNAME", "admin"), "admin username to create")
	password := flag.String("password", envOr("UPTIME_KUMA_PASSWORD", ""), "admin password to create")
	timeout := flag.Duration("timeout", 60*time.Second, "how long to keep trying")
	flag.Parse()

	if *password == "" {
		fmt.Fprintln(os.Stderr, "a password is required: pass -password or set UPTIME_KUMA_PASSWORD")
		os.Exit(2)
	}

	if err := run(*endpoint, *username, *password, *timeout); err != nil {
		fmt.Fprintf(os.Stderr, "bootstrap failed: %s\n", err)
		os.Exit(1)
	}
}

func run(endpoint, username, password string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// The setup event is one of the few that works without logging in, which is
	// the whole point: there is nobody to log in as yet.
	client, err := kuma.NewUnauthenticated(ctx, kuma.Config{
		Endpoint:   endpoint,
		Timeout:    30 * time.Second,
		MaxRetries: 5,
	})
	if err != nil {
		return fmt.Errorf("connecting to %s: %w", endpoint, err)
	}
	defer func() { _ = client.Close() }()

	need, err := client.NeedSetup(ctx)
	if err != nil {
		return fmt.Errorf("checking whether setup is needed: %w", err)
	}
	if !need {
		fmt.Printf("%s already has an account; nothing to do\n", endpoint)
		return nil
	}

	if err := client.Setup(ctx, username, password); err != nil {
		return fmt.Errorf("creating the admin user: %w", err)
	}
	fmt.Printf("created user %q on %s\n", username, endpoint)
	return nil
}

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
