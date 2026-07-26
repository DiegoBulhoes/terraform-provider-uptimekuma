package client

import (
	"context"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma/api"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma/wire"
)

// Every operation lives in internal/kuma/api as a function over api.Caller. These
// methods are the delegation, so callers keep one client with one method per
// operation and common.KumaClient stays an interface of methods.

func (c *Client) AddMonitorTag(ctx context.Context, tagID, monitorID int, value string) error {
	return api.AddMonitorTag(ctx, c, tagID, monitorID, value)
}

func (c *Client) CreateAPIKey(ctx context.Context, key wire.APIKey) (int, string, error) {
	return api.CreateAPIKey(ctx, c, key)
}

func (c *Client) CreateMaintenance(ctx context.Context, maintenance wire.Maintenance) (int, error) {
	return api.CreateMaintenance(ctx, c, maintenance)
}

func (c *Client) CreateMonitor(ctx context.Context, monitor wire.Monitor) (int, error) {
	return api.CreateMonitor(ctx, c, monitor)
}

func (c *Client) CreateStatusPage(ctx context.Context, title, slug string) (string, error) {
	return api.CreateStatusPage(ctx, c, title, slug)
}

func (c *Client) CreateTag(ctx context.Context, tag wire.Tag) (int, error) {
	return api.CreateTag(ctx, c, tag)
}

func (c *Client) DeleteAPIKey(ctx context.Context, id int) error {
	return api.DeleteAPIKey(ctx, c, id)
}

func (c *Client) DeleteDockerHost(ctx context.Context, id int) error {
	return api.DeleteDockerHost(ctx, c, id)
}

func (c *Client) DeleteIncident(ctx context.Context, slug string, id int) error {
	return api.DeleteIncident(ctx, c, slug, id)
}

func (c *Client) DeleteMaintenance(ctx context.Context, id int) error {
	return api.DeleteMaintenance(ctx, c, id)
}

func (c *Client) DeleteMonitor(ctx context.Context, id int, deleteChildren bool) error {
	return api.DeleteMonitor(ctx, c, id, deleteChildren)
}

func (c *Client) DeleteMonitorTag(ctx context.Context, tagID, monitorID int, value string) error {
	return api.DeleteMonitorTag(ctx, c, tagID, monitorID, value)
}

func (c *Client) DeleteNotification(ctx context.Context, id int) error {
	return api.DeleteNotification(ctx, c, id)
}

func (c *Client) DeleteProxy(ctx context.Context, id int) error {
	return api.DeleteProxy(ctx, c, id)
}

func (c *Client) DeleteRemoteBrowser(ctx context.Context, id int) error {
	return api.DeleteRemoteBrowser(ctx, c, id)
}

func (c *Client) DeleteStatusPage(ctx context.Context, slug string) error {
	return api.DeleteStatusPage(ctx, c, slug)
}

func (c *Client) DeleteTag(ctx context.Context, id int) error {
	return api.DeleteTag(ctx, c, id)
}

func (c *Client) EditIncident(ctx context.Context, slug string, id int, incident wire.StatusPageIncident) (*wire.StatusPageIncident, error) {
	return api.EditIncident(ctx, c, slug, id, incident)
}

func (c *Client) EditMonitorTag(ctx context.Context, tagID, monitorID int, value string) error {
	return api.EditMonitorTag(ctx, c, tagID, monitorID, value)
}

func (c *Client) GetAPIKey(ctx context.Context, id int) (*wire.APIKey, error) {
	return api.GetAPIKey(ctx, c, id)
}

func (c *Client) GetDockerHost(ctx context.Context, id int) (*wire.DockerHost, error) {
	return api.GetDockerHost(ctx, c, id)
}

func (c *Client) GetIncidentHistory(ctx context.Context, slug string) ([]wire.StatusPageIncident, error) {
	return api.GetIncidentHistory(ctx, c, slug)
}

func (c *Client) GetMaintenance(ctx context.Context, id int) (*wire.Maintenance, error) {
	return api.GetMaintenance(ctx, c, id)
}

func (c *Client) GetMaintenanceMonitors(ctx context.Context, maintenanceID int) ([]int, error) {
	return api.GetMaintenanceMonitors(ctx, c, maintenanceID)
}

func (c *Client) GetMaintenanceStatusPages(ctx context.Context, maintenanceID int) ([]int, error) {
	return api.GetMaintenanceStatusPages(ctx, c, maintenanceID)
}

func (c *Client) GetMonitor(ctx context.Context, id int) (*wire.Monitor, error) {
	return api.GetMonitor(ctx, c, id)
}

func (c *Client) GetNotification(ctx context.Context, id int) (*wire.Notification, error) {
	return api.GetNotification(ctx, c, id)
}

func (c *Client) GetProxy(ctx context.Context, id int) (*wire.Proxy, error) {
	return api.GetProxy(ctx, c, id)
}

func (c *Client) GetRemoteBrowser(ctx context.Context, id int) (*wire.RemoteBrowser, error) {
	return api.GetRemoteBrowser(ctx, c, id)
}

func (c *Client) GetSettings(ctx context.Context) (map[string]any, error) {
	return api.GetSettings(ctx, c)
}

func (c *Client) GetStatusPage(ctx context.Context, slug string) (*wire.StatusPage, error) {
	return api.GetStatusPage(ctx, c, slug)
}

func (c *Client) GetStatusPageGroups(ctx context.Context, slug string) ([]wire.StatusPageGroup, error) {
	return api.GetStatusPageGroups(ctx, c, slug)
}

func (c *Client) GetTag(ctx context.Context, id int) (*wire.Tag, error) {
	return api.GetTag(ctx, c, id)
}

func (c *Client) ListAPIKeys(ctx context.Context) (map[int]wire.APIKey, error) {
	return api.ListAPIKeys(ctx, c)
}

func (c *Client) ListDockerHosts(ctx context.Context) (map[int]wire.DockerHost, error) {
	return api.ListDockerHosts(ctx, c)
}

func (c *Client) ListMaintenances(ctx context.Context) (map[int]wire.Maintenance, error) {
	return api.ListMaintenances(ctx, c)
}

func (c *Client) ListMonitors(ctx context.Context) (map[int]wire.Monitor, error) {
	return api.ListMonitors(ctx, c)
}

func (c *Client) ListNotifications(ctx context.Context) (map[int]wire.Notification, error) {
	return api.ListNotifications(ctx, c)
}

func (c *Client) ListProxies(ctx context.Context) (map[int]wire.Proxy, error) {
	return api.ListProxies(ctx, c)
}

func (c *Client) ListRemoteBrowsers(ctx context.Context) (map[int]wire.RemoteBrowser, error) {
	return api.ListRemoteBrowsers(ctx, c)
}

func (c *Client) ListStatusPages(ctx context.Context, refresh bool) (map[int]wire.StatusPage, error) {
	return api.ListStatusPages(ctx, c, refresh)
}

func (c *Client) ListTags(ctx context.Context) ([]wire.Tag, error) {
	return api.ListTags(ctx, c)
}

func (c *Client) NeedSetup(ctx context.Context) (bool, error) {
	return api.NeedSetup(ctx, c)
}

func (c *Client) PauseMaintenance(ctx context.Context, id int) error {
	return api.PauseMaintenance(ctx, c, id)
}

func (c *Client) PauseMonitor(ctx context.Context, id int) error {
	return api.PauseMonitor(ctx, c, id)
}

func (c *Client) PostIncident(ctx context.Context, slug string, incident wire.StatusPageIncident) (*wire.StatusPageIncident, error) {
	return api.PostIncident(ctx, c, slug, incident)
}

func (c *Client) ResolveIncident(ctx context.Context, slug string, id int) error {
	return api.ResolveIncident(ctx, c, slug, id)
}

func (c *Client) ResumeMaintenance(ctx context.Context, id int) error {
	return api.ResumeMaintenance(ctx, c, id)
}

func (c *Client) ResumeMonitor(ctx context.Context, id int) error {
	return api.ResumeMonitor(ctx, c, id)
}

func (c *Client) SaveDockerHost(ctx context.Context, id *int, host wire.DockerHost) (int, error) {
	return api.SaveDockerHost(ctx, c, id, host)
}

func (c *Client) SaveNotification(ctx context.Context, id *int, payload map[string]any) (int, error) {
	return api.SaveNotification(ctx, c, id, payload)
}

func (c *Client) SaveProxy(ctx context.Context, id *int, proxy wire.Proxy) (int, error) {
	return api.SaveProxy(ctx, c, id, proxy)
}

func (c *Client) SaveRemoteBrowser(ctx context.Context, id *int, browser wire.RemoteBrowser) (int, error) {
	return api.SaveRemoteBrowser(ctx, c, id, browser)
}

func (c *Client) SaveStatusPage(ctx context.Context, slug string, config wire.StatusPage, icon string, groups []wire.StatusPageGroup) ([]wire.StatusPageGroup, error) {
	return api.SaveStatusPage(ctx, c, slug, config, icon, groups)
}

func (c *Client) SetAPIKeyActive(ctx context.Context, id int, active bool) error {
	return api.SetAPIKeyActive(ctx, c, id, active)
}

func (c *Client) SetMaintenanceMonitors(ctx context.Context, maintenanceID int, monitorIDs []int) error {
	return api.SetMaintenanceMonitors(ctx, c, maintenanceID, monitorIDs)
}

func (c *Client) SetMaintenanceStatusPages(ctx context.Context, maintenanceID int, statusPageIDs []int) error {
	return api.SetMaintenanceStatusPages(ctx, c, maintenanceID, statusPageIDs)
}

func (c *Client) SetSettings(ctx context.Context, settings map[string]any, currentPassword string) error {
	return api.SetSettings(ctx, c, settings, currentPassword)
}

func (c *Client) Setup(ctx context.Context, username, password string) error {
	return api.Setup(ctx, c, username, password)
}

func (c *Client) TestDockerHost(ctx context.Context, host wire.DockerHost) (string, error) {
	return api.TestDockerHost(ctx, c, host)
}

func (c *Client) TestNotification(ctx context.Context, payload map[string]any) error {
	return api.TestNotification(ctx, c, payload)
}

func (c *Client) UnpinIncident(ctx context.Context, slug string) error {
	return api.UnpinIncident(ctx, c, slug)
}

func (c *Client) UpdateMaintenance(ctx context.Context, maintenance wire.Maintenance) error {
	return api.UpdateMaintenance(ctx, c, maintenance)
}

func (c *Client) UpdateMonitor(ctx context.Context, monitor wire.Monitor) error {
	return api.UpdateMonitor(ctx, c, monitor)
}

func (c *Client) UpdateTag(ctx context.Context, tag wire.Tag) error {
	return api.UpdateTag(ctx, c, tag)
}
