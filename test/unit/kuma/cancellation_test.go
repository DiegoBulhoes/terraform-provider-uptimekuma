package kuma_test

import (
	"context"
	"errors"
	"testing"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
)

// A method that ignores a done context hangs Terraform until the whole run times
// out, with no indication of which resource is stuck. This client never connected,
// so returning nil means the context was not consulted at all.

func TestNoMethodIgnoresACancelledContext(t *testing.T) {
	t.Parallel()

	client := kuma.NewForHTTPTestOnly("http://127.0.0.1:1")

	cases := map[string]func(context.Context) error{
		// Push-only cache.
		"ListNotifications": func(ctx context.Context) error {
			_, err := client.ListNotifications(ctx)
			return err
		},
		"GetNotification": func(ctx context.Context) error {
			_, err := client.GetNotification(ctx, 1)
			return err
		},
		"ListProxies": func(ctx context.Context) error {
			_, err := client.ListProxies(ctx)
			return err
		},
		"GetProxy": func(ctx context.Context) error {
			_, err := client.GetProxy(ctx, 1)
			return err
		},
		"ListDockerHosts": func(ctx context.Context) error {
			_, err := client.ListDockerHosts(ctx)
			return err
		},
		"GetDockerHost": func(ctx context.Context) error {
			_, err := client.GetDockerHost(ctx, 1)
			return err
		},
		"ListRemoteBrowsers": func(ctx context.Context) error {
			_, err := client.ListRemoteBrowsers(ctx)
			return err
		},
		"GetRemoteBrowser": func(ctx context.Context) error {
			_, err := client.GetRemoteBrowser(ctx, 1)
			return err
		},

		// With a getter event.
		"ListMonitors": func(ctx context.Context) error {
			_, err := client.ListMonitors(ctx)
			return err
		},
		"GetMonitor": func(ctx context.Context) error {
			_, err := client.GetMonitor(ctx, 1)
			return err
		},
		"ListTags": func(ctx context.Context) error {
			_, err := client.ListTags(ctx)
			return err
		},
		"GetTag": func(ctx context.Context) error {
			_, err := client.GetTag(ctx, 1)
			return err
		},
		"ListMaintenances": func(ctx context.Context) error {
			_, err := client.ListMaintenances(ctx)
			return err
		},
		"GetMaintenance": func(ctx context.Context) error {
			_, err := client.GetMaintenance(ctx, 1)
			return err
		},
		"GetMaintenanceMonitors": func(ctx context.Context) error {
			_, err := client.GetMaintenanceMonitors(ctx, 1)
			return err
		},
		"GetMaintenanceStatusPages": func(ctx context.Context) error {
			_, err := client.GetMaintenanceStatusPages(ctx, 1)
			return err
		},
		"ListAPIKeys": func(ctx context.Context) error {
			_, err := client.ListAPIKeys(ctx)
			return err
		},
		"GetSettings": func(ctx context.Context) error {
			_, err := client.GetSettings(ctx)
			return err
		},
		"NeedSetup": func(ctx context.Context) error {
			_, err := client.NeedSetup(ctx)
			return err
		},
		"GetAPIKey": func(ctx context.Context) error {
			_, err := client.GetAPIKey(ctx, 1)
			return err
		},
		"GetIncidentHistory": func(ctx context.Context) error {
			_, err := client.GetIncidentHistory(ctx, "slug")
			return err
		},
		"ListStatusPages": func(ctx context.Context) error {
			_, err := client.ListStatusPages(ctx, false)
			return err
		},
		"GetStatusPage": func(ctx context.Context) error {
			_, err := client.GetStatusPage(ctx, "slug")
			return err
		},

		// Writes.
		"CreateMonitor": func(ctx context.Context) error {
			_, err := client.CreateMonitor(ctx, kuma.Monitor{Name: "n", Type: "http"})
			return err
		},
		"UpdateMonitor": func(ctx context.Context) error {
			return client.UpdateMonitor(ctx, kuma.Monitor{ID: 1, Name: "n", Type: "http"})
		},
		"DeleteMonitor": func(ctx context.Context) error {
			return client.DeleteMonitor(ctx, 1, false)
		},
		"PauseMonitor":  func(ctx context.Context) error { return client.PauseMonitor(ctx, 1) },
		"ResumeMonitor": func(ctx context.Context) error { return client.ResumeMonitor(ctx, 1) },
		"CreateTag": func(ctx context.Context) error {
			_, err := client.CreateTag(ctx, kuma.Tag{Name: "n", Color: "#fff"})
			return err
		},
		"UpdateTag": func(ctx context.Context) error {
			return client.UpdateTag(ctx, kuma.Tag{ID: 1, Name: "n", Color: "#fff"})
		},
		"DeleteTag": func(ctx context.Context) error { return client.DeleteTag(ctx, 1) },
		"AddMonitorTag": func(ctx context.Context) error {
			return client.AddMonitorTag(ctx, 1, 1, "v")
		},
		"DeleteMonitorTag": func(ctx context.Context) error {
			return client.DeleteMonitorTag(ctx, 1, 1, "v")
		},
		"SaveNotification": func(ctx context.Context) error {
			_, err := client.SaveNotification(ctx, nil, map[string]any{"name": "n", "type": "webhook"})
			return err
		},
		"DeleteNotification": func(ctx context.Context) error {
			return client.DeleteNotification(ctx, 1)
		},
		"CreateMaintenance": func(ctx context.Context) error {
			_, err := client.CreateMaintenance(ctx, kuma.Maintenance{Title: "t", Strategy: "manual"})
			return err
		},
		"UpdateMaintenance": func(ctx context.Context) error {
			return client.UpdateMaintenance(ctx, kuma.Maintenance{ID: 1, Title: "t", Strategy: "manual"})
		},
		"DeleteMaintenance": func(ctx context.Context) error {
			return client.DeleteMaintenance(ctx, 1)
		},
		"SetMaintenanceMonitors": func(ctx context.Context) error {
			return client.SetMaintenanceMonitors(ctx, 1, []int{1})
		},
		"SetMaintenanceStatusPages": func(ctx context.Context) error {
			return client.SetMaintenanceStatusPages(ctx, 1, []int{1})
		},
		"PauseMaintenance":  func(ctx context.Context) error { return client.PauseMaintenance(ctx, 1) },
		"ResumeMaintenance": func(ctx context.Context) error { return client.ResumeMaintenance(ctx, 1) },
		"SaveProxy": func(ctx context.Context) error {
			_, err := client.SaveProxy(ctx, nil, kuma.Proxy{Host: "h", Port: 1, Protocol: "http"})
			return err
		},
		"DeleteProxy": func(ctx context.Context) error { return client.DeleteProxy(ctx, 1) },
		"SaveDockerHost": func(ctx context.Context) error {
			_, err := client.SaveDockerHost(ctx, nil, kuma.DockerHost{Name: "n", DockerType: "socket", DockerDaemon: "/var/run/docker.sock"})
			return err
		},
		"DeleteDockerHost": func(ctx context.Context) error { return client.DeleteDockerHost(ctx, 1) },
		"SaveRemoteBrowser": func(ctx context.Context) error {
			_, err := client.SaveRemoteBrowser(ctx, nil, kuma.RemoteBrowser{Name: "n", URL: "ws://x"})
			return err
		},
		"DeleteRemoteBrowser": func(ctx context.Context) error {
			return client.DeleteRemoteBrowser(ctx, 1)
		},
		"CreateAPIKey": func(ctx context.Context) error {
			_, _, err := client.CreateAPIKey(ctx, kuma.APIKey{Name: "n"})
			return err
		},
		"DeleteAPIKey": func(ctx context.Context) error { return client.DeleteAPIKey(ctx, 1) },
		"SetAPIKeyActive": func(ctx context.Context) error {
			return client.SetAPIKeyActive(ctx, 1, true)
		},
		"SetSettings": func(ctx context.Context) error {
			return client.SetSettings(ctx, map[string]any{"k": "v"}, "")
		},
		"CreateStatusPage": func(ctx context.Context) error {
			_, err := client.CreateStatusPage(ctx, "title", "slug")
			return err
		},
		"SaveStatusPage": func(ctx context.Context) error {
			_, err := client.SaveStatusPage(ctx, "s", kuma.StatusPage{Slug: "s", Title: "t"}, "", nil)
			return err
		},
		"DeleteStatusPage": func(ctx context.Context) error {
			return client.DeleteStatusPage(ctx, "slug")
		},
		"PostIncident": func(ctx context.Context) error {
			_, err := client.PostIncident(ctx, "slug", kuma.StatusPageIncident{Title: "t", Content: "c"})
			return err
		},
		"UnpinIncident": func(ctx context.Context) error {
			return client.UnpinIncident(ctx, "slug")
		},
		"EditIncident": func(ctx context.Context) error {
			_, err := client.EditIncident(ctx, "slug", 1, kuma.StatusPageIncident{Title: "t", Content: "c"})
			return err
		},
		"ResolveIncident": func(ctx context.Context) error {
			return client.ResolveIncident(ctx, "slug", 1)
		},
		"DeleteIncident": func(ctx context.Context) error {
			return client.DeleteIncident(ctx, "slug", 1)
		},
		"EditMonitorTag": func(ctx context.Context) error {
			return client.EditMonitorTag(ctx, 1, 1, "v")
		},
		"TestDockerHost": func(ctx context.Context) error {
			_, err := client.TestDockerHost(ctx, kuma.DockerHost{Name: "n", DockerType: "socket", DockerDaemon: "/var/run/docker.sock"})
			return err
		},
		"TestNotification": func(ctx context.Context) error {
			return client.TestNotification(ctx, map[string]any{"type": "webhook"})
		},
		"Setup": func(ctx context.Context) error {
			return client.Setup(ctx, "admin", "password123")
		},
	}

	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(context.Background())
			cancel()

			err := call(ctx)
			if err == nil {
				t.Fatal("a cancelled context must produce an error, not a silent success")
			}
			// context.Canceled or a wrapped connection error are both fine; nil is not.
			if errors.Is(err, kuma.ErrNotFound) {
				t.Errorf("a cancelled context reported as not-found would make Terraform "+
					"delete the resource from state: %v", err)
			}
		})
	}
}

// The one read that goes over HTTP, with its own request path.
func TestGetStatusPageGroupsRejectsACancelledContext(t *testing.T) {
	t.Parallel()

	client := kuma.NewForHTTPTestOnly("http://127.0.0.1:1")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := client.GetStatusPageGroups(ctx, "slug"); err == nil {
		t.Fatal("expected an error from a cancelled context")
	}
}
