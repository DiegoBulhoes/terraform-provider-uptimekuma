//go:build integration

// Package resource_test holds the acceptance tests for the provider's managed
// resources. They run real Terraform plans against a real Uptime Kuma instance.
package resource_test

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/test/acctest"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
)

func TestMain(m *testing.M) {
	acctest.SetupTestContainer(m)
}

// checkMonitorDestroyed asserts every monitor tracked by the given resource type
// is gone from the server, not merely absent from state.
func checkMonitorDestroyed(resourceType string) func(*terraform.State) error {
	return func(state *terraform.State) error {
		client, err := acctest.GetClient()
		if err != nil {
			return err
		}

		for name, rs := range state.RootModule().Resources {
			if rs.Type != resourceType {
				continue
			}
			id, err := strconv.Atoi(rs.Primary.ID)
			if err != nil {
				return fmt.Errorf("%s has a non-numeric ID %q", name, rs.Primary.ID)
			}
			if _, err := client.GetMonitor(context.Background(), id); err == nil {
				return fmt.Errorf("monitor %d (%s) still exists", id, name)
			} else if !kuma.IsNotFound(err) {
				return fmt.Errorf("checking monitor %d: %w", id, err)
			}
		}
		return nil
	}
}

// monitorExists confirms the server really has the monitor, catching the case
// where state looks right but nothing was created.
func monitorExists(resourceName string) func(*terraform.State) error {
	return func(state *terraform.State) error {
		rs, ok := state.RootModule().Resources[resourceName]
		if !ok {
			return fmt.Errorf("%s not found in state", resourceName)
		}
		id, err := strconv.Atoi(rs.Primary.ID)
		if err != nil {
			return fmt.Errorf("%s has a non-numeric ID %q", resourceName, rs.Primary.ID)
		}

		client, err := acctest.GetClient()
		if err != nil {
			return err
		}
		if _, err := client.GetMonitor(context.Background(), id); err != nil {
			return fmt.Errorf("monitor %d does not exist on the server: %w", id, err)
		}
		return nil
	}
}
