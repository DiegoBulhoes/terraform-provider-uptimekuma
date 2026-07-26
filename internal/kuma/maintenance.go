package kuma

import (
	"context"
	"fmt"
)

// CreateMaintenance creates a maintenance window and returns its ID.
func (c *Client) CreateMaintenance(ctx context.Context, maintenance Maintenance) (int, error) {
	NormalizeMaintenance(&maintenance)

	var resp struct {
		ackEnvelope
		MaintenanceID int `json:"maintenanceID"`
	}
	if err := c.mutate(ctx, c.cache.maintenances, &resp, "addMaintenance", maintenance); err != nil {
		return 0, err
	}
	if resp.MaintenanceID == 0 {
		return 0, fmt.Errorf("server accepted the maintenance but returned no ID")
	}
	return resp.MaintenanceID, nil
}

// UpdateMaintenance saves an existing maintenance window. Like editMonitor, this
// is a whole-object write.
func (c *Client) UpdateMaintenance(ctx context.Context, maintenance Maintenance) error {
	if maintenance.ID == 0 {
		return fmt.Errorf("maintenance ID is required to update")
	}
	NormalizeMaintenance(&maintenance)
	return c.mutate(ctx, c.cache.maintenances, nil, "editMaintenance", maintenance)
}

// GetMaintenance fetches one maintenance window; the payload comes back in the
// acknowledgement.
func (c *Client) GetMaintenance(ctx context.Context, id int) (*Maintenance, error) {
	var resp struct {
		ackEnvelope
		Maintenance *Maintenance `json:"maintenance"`
	}
	if err := c.call(ctx, &resp, "getMaintenance", id); err != nil {
		return nil, err
	}
	if resp.Maintenance == nil {
		return nil, ErrNotFound
	}
	return resp.Maintenance, nil
}

// ListMaintenances returns every maintenance window.
func (c *Client) ListMaintenances(ctx context.Context) (map[int]Maintenance, error) {
	refresh := func(ctx context.Context) error {
		return c.refreshList(ctx, c.cache.maintenances, "getMaintenanceList")
	}
	if err := c.ensureLoaded(ctx, c.cache.maintenances, refresh); err != nil {
		return nil, err
	}
	return c.cache.maintenances.all(), nil
}

// DeleteMaintenance removes a maintenance window.
func (c *Client) DeleteMaintenance(ctx context.Context, id int) error {
	return c.mutate(ctx, c.cache.maintenances, nil, "deleteMaintenance", id)
}

// PauseMaintenance suspends a maintenance window.
func (c *Client) PauseMaintenance(ctx context.Context, id int) error {
	return c.mutate(ctx, c.cache.maintenances, nil, "pauseMaintenance", id)
}

// ResumeMaintenance reactivates a maintenance window.
func (c *Client) ResumeMaintenance(ctx context.Context, id int) error {
	return c.mutate(ctx, c.cache.maintenances, nil, "resumeMaintenance", id)
}

// SetMaintenanceMonitors sets which monitors the window applies to.
//
// This replaces the whole association set rather than adding to it, so callers
// must always send the complete list.
func (c *Client) SetMaintenanceMonitors(ctx context.Context, maintenanceID int, monitorIDs []int) error {
	monitors := make([]map[string]any, 0, len(monitorIDs))
	for _, id := range monitorIDs {
		monitors = append(monitors, map[string]any{"id": id})
	}
	return c.call(ctx, nil, "addMonitorMaintenance", maintenanceID, monitors)
}

// GetMaintenanceMonitors returns the monitor IDs attached to a window.
func (c *Client) GetMaintenanceMonitors(ctx context.Context, maintenanceID int) ([]int, error) {
	var resp struct {
		ackEnvelope
		Monitors []struct {
			ID int `json:"id"`
		} `json:"monitors"`
	}
	if err := c.call(ctx, &resp, "getMonitorMaintenance", maintenanceID); err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(resp.Monitors))
	for _, monitor := range resp.Monitors {
		ids = append(ids, monitor.ID)
	}
	return ids, nil
}

// NormalizeMaintenance fills in the fields jsonToBean dereferences without
// guarding. dateRange is indexed unconditionally, so a missing array throws on
// the server even for strategies that ignore dates.
func NormalizeMaintenance(maintenance *Maintenance) {
	// The `active` column is NOT NULL with no default, and jsonToBean copies the
	// field straight through, so an unset value fails the insert.
	if maintenance.Active == nil {
		maintenance.Active = BoolPtr(true)
	}
	for len(maintenance.DateRange) < 2 {
		maintenance.DateRange = append(maintenance.DateRange, nil)
	}
	if maintenance.Weekdays == nil {
		maintenance.Weekdays = []int{}
	}
	if maintenance.DaysOfMonth == nil {
		maintenance.DaysOfMonth = []any{}
	}
	// Recurring strategies index timeRange the same way dateRange is indexed.
	if len(maintenance.TimeRange) == 0 {
		maintenance.TimeRange = []TimePart{
			{Hours: 0, Minutes: 0},
			{Hours: 0, Minutes: 0},
		}
	}
	if maintenance.TimezoneOption == nil {
		sameAsServer := "SAME_AS_SERVER"
		maintenance.TimezoneOption = &sameAsServer
	}
}
