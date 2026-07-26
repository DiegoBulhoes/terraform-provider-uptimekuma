// Package common holds the interface the resources and data sources talk to,
// plus the helpers they share.
package common

import (
	"context"
	"fmt"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
)

// KumaClient is every operation the resources and data sources need.
//
// The concrete implementation lives in internal/kuma; this interface exists so
// unit tests can drive resources with a GoMock double instead of a live server.
type KumaClient interface {
	// Monitors.
	CreateMonitor(ctx context.Context, monitor kuma.Monitor) (int, error)
	UpdateMonitor(ctx context.Context, monitor kuma.Monitor) error
	GetMonitor(ctx context.Context, id int) (*kuma.Monitor, error)
	ListMonitors(ctx context.Context) (map[int]kuma.Monitor, error)
	DeleteMonitor(ctx context.Context, id int, deleteChildren bool) error
	PauseMonitor(ctx context.Context, id int) error
	ResumeMonitor(ctx context.Context, id int) error

	// Monitor/tag associations.
	AddMonitorTag(ctx context.Context, tagID, monitorID int, value string) error
	EditMonitorTag(ctx context.Context, tagID, monitorID int, value string) error
	DeleteMonitorTag(ctx context.Context, tagID, monitorID int, value string) error

	// Tags.
	CreateTag(ctx context.Context, tag kuma.Tag) (int, error)
	UpdateTag(ctx context.Context, tag kuma.Tag) error
	GetTag(ctx context.Context, id int) (*kuma.Tag, error)
	ListTags(ctx context.Context) ([]kuma.Tag, error)
	DeleteTag(ctx context.Context, id int) error

	// Notifications.
	SaveNotification(ctx context.Context, id *int, payload map[string]any) (int, error)
	GetNotification(ctx context.Context, id int) (*kuma.Notification, error)
	ListNotifications(ctx context.Context) (map[int]kuma.Notification, error)
	DeleteNotification(ctx context.Context, id int) error

	// Maintenance windows.
	CreateMaintenance(ctx context.Context, maintenance kuma.Maintenance) (int, error)
	UpdateMaintenance(ctx context.Context, maintenance kuma.Maintenance) error
	GetMaintenance(ctx context.Context, id int) (*kuma.Maintenance, error)
	ListMaintenances(ctx context.Context) (map[int]kuma.Maintenance, error)
	DeleteMaintenance(ctx context.Context, id int) error
	PauseMaintenance(ctx context.Context, id int) error
	ResumeMaintenance(ctx context.Context, id int) error
	SetMaintenanceMonitors(ctx context.Context, maintenanceID int, monitorIDs []int) error
	GetMaintenanceMonitors(ctx context.Context, maintenanceID int) ([]int, error)

	// Status pages. Reading the group tree goes over HTTP, because no event
	// exposes it.
	CreateStatusPage(ctx context.Context, title, slug string) (string, error)
	SaveStatusPage(ctx context.Context, slug string, config kuma.StatusPage, icon string, groups []kuma.StatusPageGroup) ([]kuma.StatusPageGroup, error)
	GetStatusPage(ctx context.Context, slug string) (*kuma.StatusPage, error)
	GetStatusPageGroups(ctx context.Context, slug string) ([]kuma.StatusPageGroup, error)
	ListStatusPages(ctx context.Context, refresh bool) (map[int]kuma.StatusPage, error)
	DeleteStatusPage(ctx context.Context, slug string) error

	// Status page incidents.
	PostIncident(ctx context.Context, slug string, incident kuma.StatusPageIncident) (*kuma.StatusPageIncident, error)
	EditIncident(ctx context.Context, slug string, id int, incident kuma.StatusPageIncident) (*kuma.StatusPageIncident, error)
	ResolveIncident(ctx context.Context, slug string, id int) error
	DeleteIncident(ctx context.Context, slug string, id int) error
	UnpinIncident(ctx context.Context, slug string) error
	GetIncidentHistory(ctx context.Context, slug string) ([]kuma.StatusPageIncident, error)

	// Maintenance/status page associations.
	SetMaintenanceStatusPages(ctx context.Context, maintenanceID int, statusPageIDs []int) error
	GetMaintenanceStatusPages(ctx context.Context, maintenanceID int) ([]int, error)

	// Proxies.
	SaveProxy(ctx context.Context, id *int, proxy kuma.Proxy) (int, error)
	GetProxy(ctx context.Context, id int) (*kuma.Proxy, error)
	ListProxies(ctx context.Context) (map[int]kuma.Proxy, error)
	DeleteProxy(ctx context.Context, id int) error

	// Docker hosts.
	SaveDockerHost(ctx context.Context, id *int, host kuma.DockerHost) (int, error)
	GetDockerHost(ctx context.Context, id int) (*kuma.DockerHost, error)
	ListDockerHosts(ctx context.Context) (map[int]kuma.DockerHost, error)
	DeleteDockerHost(ctx context.Context, id int) error

	// Remote browsers.
	SaveRemoteBrowser(ctx context.Context, id *int, browser kuma.RemoteBrowser) (int, error)
	GetRemoteBrowser(ctx context.Context, id int) (*kuma.RemoteBrowser, error)
	ListRemoteBrowsers(ctx context.Context) (map[int]kuma.RemoteBrowser, error)
	DeleteRemoteBrowser(ctx context.Context, id int) error

	// API keys.
	CreateAPIKey(ctx context.Context, key kuma.APIKey) (int, string, error)
	GetAPIKey(ctx context.Context, id int) (*kuma.APIKey, error)
	ListAPIKeys(ctx context.Context) (map[int]kuma.APIKey, error)
	SetAPIKeyActive(ctx context.Context, id int, active bool) error
	DeleteAPIKey(ctx context.Context, id int) error

	// Settings and server metadata.
	GetSettings(ctx context.Context) (map[string]any, error)
	SetSettings(ctx context.Context, settings map[string]any, currentPassword string) error
	Info() kuma.ServerInfo
}

// Compile-time check that the real client satisfies the interface.
var _ KumaClient = (*kuma.Client)(nil)

// ConfigureClient extracts the client from provider data. Used by every resource
// and data source in Configure().
func ConfigureClient(providerData any) (KumaClient, error) {
	client, ok := providerData.(KumaClient)
	if !ok {
		return nil, fmt.Errorf("expected common.KumaClient, got: %T", providerData)
	}
	return client, nil
}
