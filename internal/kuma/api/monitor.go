package api

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma/wire"
)

// CreateMonitor creates a monitor and returns its ID.
//
// The event is called "add", not "addMonitor" (server/server.js:743).
func CreateMonitor(ctx context.Context, c Caller, monitor wire.Monitor) (int, error) {
	NormalizeMonitor(&monitor)

	var resp struct {
		wire.AckEnvelope
		MonitorID int `json:"monitorID"`
	}
	if err := c.Call(ctx, &resp, "add", monitor); err != nil {
		return 0, err
	}
	if resp.MonitorID == 0 {
		return 0, fmt.Errorf("server accepted the monitor but returned no ID")
	}
	return resp.MonitorID, nil
}

// UpdateMonitor saves an existing monitor.
//
// editMonitor is a whole-object write, not a patch: the server imports every
// field it receives, so a partial payload would clear the omitted columns.
// Callers must start from GetMonitor and modify that.
func UpdateMonitor(ctx context.Context, c Caller, monitor wire.Monitor) error {
	if monitor.ID == 0 {
		return fmt.Errorf("monitor ID is required to update")
	}
	NormalizeMonitor(&monitor)
	return c.Call(ctx, nil, "editMonitor", monitor)
}

// GetMonitor fetches a single monitor. Unlike the list getters, this one returns
// the payload in the acknowledgement.
func GetMonitor(ctx context.Context, c Caller, id int) (*wire.Monitor, error) {
	var resp struct {
		wire.AckEnvelope
		Monitor *wire.Monitor `json:"monitor"`
	}
	if err := c.Call(ctx, &resp, "getMonitor", id); err != nil {
		return nil, err
	}
	if resp.Monitor == nil {
		return nil, wire.ErrNotFound
	}
	return resp.Monitor, nil
}

// ListMonitors returns every monitor visible to the authenticated user.
func ListMonitors(ctx context.Context, c Caller) (map[int]wire.Monitor, error) {
	refresh := func(ctx context.Context) error {
		return c.RefreshList(ctx, c.Cache().Monitors, "getMonitorList")
	}
	if err := c.EnsureLoaded(ctx, c.Cache().Monitors, refresh); err != nil {
		return nil, err
	}
	return c.Cache().Monitors.All(), nil
}

// DeleteMonitor removes a monitor. deleteChildren decides what happens to the
// children of a group monitor.
func DeleteMonitor(ctx context.Context, c Caller, id int, deleteChildren bool) error {
	return c.Call(ctx, nil, "deleteMonitor", id, deleteChildren)
}

// PauseMonitor stops checks without deleting the monitor.
func PauseMonitor(ctx context.Context, c Caller, id int) error {
	return c.Call(ctx, nil, "pauseMonitor", id)
}

// ResumeMonitor restarts a paused monitor.
func ResumeMonitor(ctx context.Context, c Caller, id int) error {
	return c.Call(ctx, nil, "resumeMonitor", id)
}

// AddMonitorTag attaches a tag to a monitor, with an optional per-monitor value.
func AddMonitorTag(ctx context.Context, c Caller, tagID, monitorID int, value string) error {
	return c.Call(ctx, nil, "addMonitorTag", tagID, monitorID, value)
}

// EditMonitorTag updates the value of an already attached tag.
func EditMonitorTag(ctx context.Context, c Caller, tagID, monitorID int, value string) error {
	return c.Call(ctx, nil, "editMonitorTag", tagID, monitorID, value)
}

// DeleteMonitorTag detaches a tag. The value is part of the identity of the
// association, so it must match what was stored.
func DeleteMonitorTag(ctx context.Context, c Caller, tagID, monitorID int, value string) error {
	return c.Call(ctx, nil, "deleteMonitorTag", tagID, monitorID, value)
}

// MinIntervalSeconds is the smallest interval the server accepts, for both
// `interval` and `retryInterval` (wire.Monitor.validate in server/model/monitor.js).
const MinIntervalSeconds = 1

// NormalizeMonitor fills in the fields the server dereferences or validates
// without a default, so a valid Terraform config cannot produce a JavaScript
// TypeError or a validation error about a field the user never set.
func NormalizeMonitor(monitor *wire.Monitor) {
	// `add` iterates accepted_statuscodes before storing and rejects any
	// non-string entry (server/server.js:751). An absent array throws.
	if monitor.AcceptedStatusCodes == nil {
		monitor.AcceptedStatusCodes = []string{"200-299"}
	}
	// validate() rejects a retry interval below the minimum, and there is no
	// server-side default. The web UI mirrors the check interval, so do the
	// same rather than inventing a number.
	if monitor.RetryInterval < MinIntervalSeconds {
		if monitor.Interval >= MinIntervalSeconds {
			monitor.RetryInterval = monitor.Interval
		} else {
			monitor.RetryInterval = MinIntervalSeconds
		}
	}
	// Both are JSON.stringify'd unconditionally by the handler.
	if monitor.Conditions == nil {
		monitor.Conditions = json.RawMessage("[]")
	}
	if monitor.NotificationIDList == nil {
		monitor.NotificationIDList = map[string]bool{}
	}
	// Push monitors need a token, and the server never generates one — the web
	// UI does it client-side (src/pages/EditMonitor.vue) and sends it along, so
	// the provider has to do the same or the push URL is unusable.
	if monitor.Type == "push" && (monitor.PushToken == nil || *monitor.PushToken == "") {
		token, err := generatePushToken()
		if err == nil {
			monitor.PushToken = &token
		}
	}
}

// pushTokenLength matches the length the Uptime Kuma web UI uses.
const pushTokenLength = 32

// pushTokenAlphabet mirrors the character set of the UI's genSecret helper.
const pushTokenAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

// generatePushToken creates the secret embedded in a push monitor's URL.
func generatePushToken() (string, error) {
	token := make([]byte, pushTokenLength)
	limit := big.NewInt(int64(len(pushTokenAlphabet)))
	for i := range token {
		index, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", fmt.Errorf("generating push token: %w", err)
		}
		token[i] = pushTokenAlphabet[index.Int64()]
	}
	return string(token), nil
}
