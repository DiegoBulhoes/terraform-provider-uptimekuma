//go:build integration

// Package acctest provides shared test infrastructure for acceptance tests
// across all packages (kuma, provider, resource, datasource).
package acctest

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// Test credentials for the container-local admin account. The instance is
// created fresh per run and thrown away, so these are not secrets.
const (
	TestUsername = "acctest"
	TestPassword = "acctest-Passw0rd!"
)

// ProviderFactories returns the in-process provider server the acceptance tests
// run against, so no provider binary has to be built or installed.
func ProviderFactories() map[string]func() (tfprotov6.ProviderServer, error) {
	return map[string]func() (tfprotov6.ProviderServer, error){
		"uptimekuma": providerserver.NewProtocol6WithError(provider.New("acctest")()),
	}
}

// ProviderConfig is the provider block every acceptance test config starts with.
// Credentials come from the environment the container bootstrap exported.
func ProviderConfig() string {
	return fmt.Sprintf(`
provider "uptimekuma" {
  endpoint = %q
  username = %q
  password = %q
}
`,
		os.Getenv("UPTIME_KUMA_URL"),
		GetEnvOrDefault("UPTIME_KUMA_USERNAME", TestUsername),
		GetEnvOrDefault("UPTIME_KUMA_PASSWORD", TestPassword),
	)
}

var (
	client     *kuma.Client
	clientErr  error
	clientOnce sync.Once
)

// GetClient returns a shared client for test helpers such as CheckDestroy.
func GetClient() (*kuma.Client, error) {
	clientOnce.Do(func() {
		client, clientErr = kuma.New(context.Background(), kuma.Config{
			Endpoint:   os.Getenv("UPTIME_KUMA_URL"),
			Username:   GetEnvOrDefault("UPTIME_KUMA_USERNAME", TestUsername),
			Password:   GetEnvOrDefault("UPTIME_KUMA_PASSWORD", TestPassword),
			Timeout:    30 * time.Second,
			MaxRetries: 2,
		})
	})
	return client, clientErr
}

// GetEnvOrDefault returns the value of the environment variable or the fallback.
func GetEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// PreCheck verifies the test instance is reachable before running tests.
func PreCheck(t *testing.T) {
	t.Helper()
	c, err := GetClient()
	if err != nil {
		t.Fatalf("Failed to connect to the test Uptime Kuma instance: %s", err)
	}
	if _, err := c.ListMonitors(context.Background()); err != nil {
		t.Fatalf("Failed to query the test Uptime Kuma instance: %s", err)
	}
}

// SetupTestContainer starts an Uptime Kuma container, creates the admin user and
// exports the connection environment. Call this from TestMain in each test
// package that needs acceptance tests. If UPTIME_KUMA_URL is already set, that
// instance is used instead of starting a container.
//
// The work happens in runTests because os.Exit skips deferred calls: exiting
// from here directly would leak a container on every run.
func SetupTestContainer(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	if endpoint := os.Getenv("UPTIME_KUMA_URL"); endpoint != "" {
		// An external instance may still be freshly started, so run the
		// bootstrap: needSetup makes it a no-op once an account exists.
		if err := bootstrap(context.Background(), endpoint); err != nil {
			fmt.Fprintf(os.Stderr, "failed to bootstrap %s: %s\n", endpoint, err)
			return 1
		}
		return m.Run()
	}

	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        GetEnvOrDefault("UPTIME_KUMA_IMAGE", "louislam/uptime-kuma:2.4.0"),
		ExposedPorts: []string{"3001/tcp"},
		Env: map[string]string{
			// Uptime Kuma 2.x boots into a database-selection step and parks
			// there ("Waiting for user action…") with only a stub HTTP server —
			// the real Socket.IO server does not exist yet. Setting the DB type
			// up front skips that step, which is the only way to reach the API
			// unattended.
			"UPTIME_KUMA_DB_TYPE": "sqlite",
		},
		// Waiting on the HTTP root is not enough: the setup-database stub also
		// answers it. The handshake endpoint only responds once the real
		// Socket.IO server is up, after migrations.
		WaitingFor: wait.ForHTTP("/socket.io/?EIO=4&transport=polling").
			WithPort("3001/tcp").
			WithStartupTimeout(3 * time.Minute),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to start uptime kuma container: %s\n", err)
		return 1
	}

	defer func() {
		if err := container.Terminate(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "failed to terminate uptime kuma container: %s\n", err)
		}
	}()

	host, err := container.Host(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get container host: %s\n", err)
		return 1
	}
	port, err := container.MappedPort(ctx, "3001/tcp")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get container port: %s\n", err)
		return 1
	}

	endpoint := fmt.Sprintf("http://%s:%s", host, port.Port())

	if err := bootstrap(ctx, endpoint); err != nil {
		fmt.Fprintf(os.Stderr, "failed to bootstrap uptime kuma: %s\n", err)
		return 1
	}

	failed := false
	setEnv := func(key, value string) {
		if err := os.Setenv(key, value); err != nil {
			fmt.Fprintf(os.Stderr, "failed to set %s: %s\n", key, err)
			failed = true
		}
	}
	setEnv("UPTIME_KUMA_URL", endpoint)
	setEnv("UPTIME_KUMA_USERNAME", TestUsername)
	setEnv("UPTIME_KUMA_PASSWORD", TestPassword)
	if failed {
		return 1
	}

	return m.Run()
}

// bootstrap creates the admin user on a fresh instance.
//
// A brand-new Uptime Kuma has no account at all, so nothing can be done until
// `setup` runs — and `setup` is one of the few events that works without a
// login, which is what NewUnauthenticated is for.
func bootstrap(ctx context.Context, endpoint string) error {
	cfg := kuma.Config{
		Endpoint:   endpoint,
		Timeout:    30 * time.Second,
		MaxRetries: 5,
	}

	client, err := kuma.NewUnauthenticated(ctx, cfg)
	if err != nil {
		return fmt.Errorf("connecting for bootstrap: %w", err)
	}
	defer client.Close()

	need, err := client.NeedSetup(ctx)
	if err != nil {
		return fmt.Errorf("checking setup status: %w", err)
	}
	if !need {
		return nil
	}
	if err := client.Setup(ctx, TestUsername, TestPassword); err != nil {
		return fmt.Errorf("creating the admin user: %w", err)
	}
	return nil
}
