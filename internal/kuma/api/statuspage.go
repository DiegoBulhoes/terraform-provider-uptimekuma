package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma/wire"
)

// Status pages are the one entity where Socket.IO is not enough.
//
// `getStatusPage` returns the page's configuration but not its groups, and no
// event returns them. The only source is the public HTTP route
// `GET /api/status-page/:slug`, so reads of the group tree go over HTTP while
// everything else stays on Socket.IO.

// CreateStatusPage creates a page and returns its slug.
//
// addStatusPage takes only a title and a slug; everything else is applied by a
// follow-up SaveStatusPage. The server lowercases the slug, so the value it
// returns is the one to keep.
func CreateStatusPage(ctx context.Context, c Caller, title, slug string) (string, error) {
	var resp struct {
		wire.AckEnvelope
		Slug string `json:"slug"`
	}
	if err := c.Call(ctx, &resp, "addStatusPage", title, slug); err != nil {
		return "", err
	}
	if resp.Slug == "" {
		return "", fmt.Errorf("server accepted the status page but returned no slug")
	}
	return resp.Slug, nil
}

// SaveStatusPage writes the configuration and the whole group tree.
//
// This is a save-everything call: the groups sent here replace what the page
// had, and any group missing from the list is deleted. The returned list carries
// the IDs the server assigned.
//
// icon is the imgDataUrl argument. A `data:image/png;base64,...` value is stored
// as a file; anything else is treated as a URL and kept verbatim.
//
// Renaming is possible: config.Slug may differ from the slug argument, which is
// the current one.
func SaveStatusPage(
	ctx context.Context,
	c Caller,
	slug string,
	config wire.StatusPage,
	icon string,
	groups []wire.StatusPageGroup,
) ([]wire.StatusPageGroup, error) {
	normalizeStatusPage(&config)

	// Never nil: the handler iterates the list, and deleting every group is how
	// a page is emptied.
	if groups == nil {
		groups = []wire.StatusPageGroup{}
	}

	var resp struct {
		wire.AckEnvelope
		PublicGroupList []wire.StatusPageGroup `json:"publicGroupList"`
	}
	if err := c.Call(ctx, &resp, "saveStatusPage", slug, config, icon, groups); err != nil {
		return nil, err
	}
	return resp.PublicGroupList, nil
}

// GetStatusPage reads a page's configuration. The group tree is not included;
// use GetStatusPageGroups for that.
func GetStatusPage(ctx context.Context, c Caller, slug string) (*wire.StatusPage, error) {
	var resp struct {
		wire.AckEnvelope
		Config *wire.StatusPage `json:"config"`
	}
	if err := c.Call(ctx, &resp, "getStatusPage", slug); err != nil {
		return nil, err
	}
	if resp.Config == nil {
		return nil, wire.ErrNotFound
	}
	return resp.Config, nil
}

// GetStatusPageGroups reads the group tree over HTTP, the only place it is
// exposed.
//
// The route is cached for five minutes server-side. That is harmless right after
// a write, because saveStatusPage clears the wire.Cache, but a plain refresh can see
// values up to five minutes old.
func GetStatusPageGroups(ctx context.Context, c Caller, slug string) ([]wire.StatusPageGroup, error) {
	endpoint, err := url.Parse(c.Endpoint())
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint %q: %w", c.Endpoint(), err)
	}
	endpoint.Path = "/api/status-page/" + url.PathEscape(strings.ToLower(slug))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}

	resp, err := c.HTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("reading status page groups: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("reading status page groups: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// The route answers 404 with a JSON body for an unknown slug.
		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("status page %q: %w", slug, wire.ErrNotFound)
		}
		return nil, fmt.Errorf("reading status page groups: unexpected status %s", resp.Status)
	}

	var payload struct {
		PublicGroupList []wire.StatusPageGroup `json:"publicGroupList"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decoding status page groups: %w", err)
	}
	return payload.PublicGroupList, nil
}

// ListStatusPages returns every status page, keyed by ID.
//
// Unlike every other pushed list, this one is sent exactly once — during
// afterLogin — and no mutation re-sends it (sendStatusPageList has a single call
// site in server/server.js). A page created during this session therefore will
// not appear here.
//
// refresh forces a reconnect so the server runs afterLogin again, which is the
// only way to get a current list. It costs one login, and the server allows 20
// per minute, so callers should not do it in a loop.
func ListStatusPages(ctx context.Context, c Caller, refresh bool) (map[int]wire.StatusPage, error) {
	if refresh {
		// Dropping the wire.Cache is enough: ensureLoaded reconnects when a
		// push-only list is missing, and afterLogin then resends it.
		c.Cache().StatusPages.Invalidate()
	}
	if err := c.EnsureLoaded(ctx, c.Cache().StatusPages, nil); err != nil {
		return nil, err
	}
	return c.Cache().StatusPages.All(), nil
}

// DeleteStatusPage removes a page along with its groups and incidents.
func DeleteStatusPage(ctx context.Context, c Caller, slug string) error {
	// Not routed through mutate: no push follows this, so there is nothing to
	// wait for. The cached list is dropped instead, so a later read reconnects.
	if err := c.Call(ctx, nil, "deleteStatusPage", slug); err != nil {
		return err
	}
	c.Cache().StatusPages.Invalidate()
	return nil
}

// PostIncident creates or updates the pinned incident of a status page.
//
// A page holds at most one pinned incident: posting always pins the result, so
// this doubles as "replace the current banner".
func PostIncident(ctx context.Context, c Caller, slug string, incident wire.StatusPageIncident) (*wire.StatusPageIncident, error) {
	var resp struct {
		wire.AckEnvelope
		Incident *wire.StatusPageIncident `json:"incident"`
	}
	if err := c.Call(ctx, &resp, "postIncident", slug, incident); err != nil {
		return nil, err
	}
	if resp.Incident == nil {
		return nil, fmt.Errorf("server accepted the incident but returned nothing")
	}
	return resp.Incident, nil
}

// EditIncident updates an incident without changing its pinned state.
func EditIncident(ctx context.Context, c Caller, slug string, id int, incident wire.StatusPageIncident) (*wire.StatusPageIncident, error) {
	var resp struct {
		wire.AckEnvelope
		Incident *wire.StatusPageIncident `json:"incident"`
	}
	if err := c.Call(ctx, &resp, "editIncident", slug, id, incident); err != nil {
		return nil, err
	}
	return resp.Incident, nil
}

// ResolveIncident marks an incident resolved, which also unpins it.
func ResolveIncident(ctx context.Context, c Caller, slug string, id int) error {
	return c.Call(ctx, nil, "resolveIncident", slug, id)
}

// DeleteIncident removes an incident outright.
func DeleteIncident(ctx context.Context, c Caller, slug string, id int) error {
	return c.Call(ctx, nil, "deleteIncident", slug, id)
}

// UnpinIncident hides the banner without deleting the incident.
func UnpinIncident(ctx context.Context, c Caller, slug string) error {
	return c.Call(ctx, nil, "unpinIncident", slug)
}

// GetIncidentHistory returns the incidents of a page, newest first.
func GetIncidentHistory(ctx context.Context, c Caller, slug string) ([]wire.StatusPageIncident, error) {
	var resp struct {
		wire.AckEnvelope
		Incidents []wire.StatusPageIncident `json:"incidents"`
	}
	// The cursor argument pages through the history; nil asks for the first page.
	if err := c.Call(ctx, &resp, "getIncidentHistory", slug, nil); err != nil {
		return nil, err
	}
	return resp.Incidents, nil
}

// SetMaintenanceStatusPages sets which status pages a maintenance window shows
// on. Like the monitor association, this replaces the whole set.
func SetMaintenanceStatusPages(ctx context.Context, c Caller, maintenanceID int, statusPageIDs []int) error {
	pages := make([]map[string]any, 0, len(statusPageIDs))
	for _, id := range statusPageIDs {
		pages = append(pages, map[string]any{"id": id})
	}
	return c.Call(ctx, nil, "addMaintenanceStatusPage", maintenanceID, pages)
}

// GetMaintenanceStatusPages returns the status page IDs attached to a window.
func GetMaintenanceStatusPages(ctx context.Context, c Caller, maintenanceID int) ([]int, error) {
	var resp struct {
		wire.AckEnvelope
		StatusPages []struct {
			ID int `json:"id"`
		} `json:"statusPages"`
	}
	if err := c.Call(ctx, &resp, "getMaintenanceStatusPage", maintenanceID); err != nil {
		return nil, err
	}
	ids := make([]int, 0, len(resp.StatusPages))
	for _, page := range resp.StatusPages {
		ids = append(ids, page.ID)
	}
	return ids, nil
}

// normalizeStatusPage fills in the fields saveStatusPage reads without a
// fallback.
func normalizeStatusPage(page *wire.StatusPage) {
	// updateDomainNameList bails out unless this is an array, leaving the
	// existing domains untouched instead of clearing them.
	if page.DomainNameList == nil {
		page.DomainNameList = []string{}
	}
	if page.Theme == "" {
		page.Theme = "auto"
	}
}
