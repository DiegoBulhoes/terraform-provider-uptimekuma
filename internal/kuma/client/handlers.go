package client

import (
	"encoding/json"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma/wire"

	socketio "github.com/maldikhan/go.socket.io/socket.io/v5/client"
)

// Handlers for the lists the server pushes. Registered before connecting,
// because the server starts pushing right after login.

// registerHandlers subscribes to the server-pushed lists. This has to happen
// before connecting, because the server starts pushing right after login.
func (c *Client) registerHandlers(sio *socketio.Client) {
	sio.On("disconnect", func([]any) {
		c.markUnhealthy()
	})

	// monitorList and maintenanceList arrive as objects keyed by ID; the rest
	// arrive as arrays.
	sio.On("monitorList", func(in []any) {
		if items, ok := wire.DecodeKeyedList[wire.Monitor](in); ok {
			c.cache.Monitors.Replace(items)
		}
	})
	sio.On("updateMonitorIntoList", func(in []any) {
		if items, ok := wire.DecodeKeyedList[wire.Monitor](in); ok {
			c.cache.Monitors.Patch(items)
		}
	})
	sio.On("deleteMonitorFromList", func(in []any) {
		if id, ok := wire.DecodeInt(in); ok {
			c.cache.Monitors.Remove(id)
		}
	})
	sio.On("maintenanceList", func(in []any) {
		if items, ok := wire.DecodeKeyedList[wire.Maintenance](in); ok {
			c.cache.Maintenances.Replace(items)
		}
	})
	sio.On("notificationList", func(in []any) {
		if items, ok := wire.DecodeArrayList(in, func(n wire.Notification) int { return n.ID }); ok {
			c.cache.Notifications.Replace(items)
		}
	})
	sio.On("proxyList", func(in []any) {
		if items, ok := wire.DecodeArrayList(in, func(p wire.Proxy) int { return p.ID }); ok {
			c.cache.Proxies.Replace(items)
		}
	})
	sio.On("dockerHostList", func(in []any) {
		if items, ok := wire.DecodeArrayList(in, func(d wire.DockerHost) int { return d.ID }); ok {
			c.cache.DockerHosts.Replace(items)
		}
	})
	sio.On("remoteBrowserList", func(in []any) {
		if items, ok := wire.DecodeArrayList(in, func(r wire.RemoteBrowser) int { return r.ID }); ok {
			c.cache.RemoteBrowsers.Replace(items)
		}
	})
	sio.On("apiKeyList", func(in []any) {
		if items, ok := wire.DecodeArrayList(in, func(k wire.APIKey) int { return k.ID }); ok {
			c.cache.APIKeys.Replace(items)
		}
	})
	sio.On("statusPageList", func(in []any) {
		if items, ok := wire.DecodeKeyedList[wire.StatusPage](in); ok {
			c.cache.StatusPages.Replace(items)
		}
	})
	sio.On("info", func(in []any) {
		var info wire.ServerInfo
		if raw, ok := wire.FirstRaw(in); ok && json.Unmarshal(raw, &info) == nil {
			c.cache.SetInfo(info)
		}
	})
}
