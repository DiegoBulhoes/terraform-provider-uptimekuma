package kuma

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
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
func (c *Client) CreateStatusPage(ctx context.Context, title, slug string) (string, error) {
	var resp struct {
		ackEnvelope
		Slug string `json:"slug"`
	}
	if err := c.call(ctx, &resp, "addStatusPage", title, slug); err != nil {
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
func (c *Client) SaveStatusPage(
	ctx context.Context,
	slug string,
	config StatusPage,
	icon string,
	groups []StatusPageGroup,
) ([]StatusPageGroup, error) {
	normalizeStatusPage(&config)

	// Never nil: the handler iterates the list, and deleting every group is how
	// a page is emptied.
	if groups == nil {
		groups = []StatusPageGroup{}
	}

	var resp struct {
		ackEnvelope
		PublicGroupList []StatusPageGroup `json:"publicGroupList"`
	}
	if err := c.call(ctx, &resp, "saveStatusPage", slug, config, icon, groups); err != nil {
		return nil, err
	}
	return resp.PublicGroupList, nil
}

// GetStatusPage reads a page's configuration. The group tree is not included;
// use GetStatusPageGroups for that.
func (c *Client) GetStatusPage(ctx context.Context, slug string) (*StatusPage, error) {
	var resp struct {
		ackEnvelope
		Config *StatusPage `json:"config"`
	}
	if err := c.call(ctx, &resp, "getStatusPage", slug); err != nil {
		return nil, err
	}
	if resp.Config == nil {
		return nil, ErrNotFound
	}
	return resp.Config, nil
}

// GetStatusPageGroups reads the group tree over HTTP, the only place it is
// exposed.
//
// The route is cached for five minutes server-side. That is harmless right after
// a write, because saveStatusPage clears the cache, but a plain refresh can see
// values up to five minutes old.
func (c *Client) GetStatusPageGroups(ctx context.Context, slug string) ([]StatusPageGroup, error) {
	endpoint, err := url.Parse(c.cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid endpoint %q: %w", c.cfg.Endpoint, err)
	}
	endpoint.Path = "/api/status-page/" + url.PathEscape(strings.ToLower(slug))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}

	resp, err := c.http().Do(req)
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
			return nil, fmt.Errorf("status page %q: %w", slug, ErrNotFound)
		}
		return nil, fmt.Errorf("reading status page groups: unexpected status %s", resp.Status)
	}

	var payload struct {
		PublicGroupList []StatusPageGroup `json:"publicGroupList"`
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
func (c *Client) ListStatusPages(ctx context.Context, refresh bool) (map[int]StatusPage, error) {
	if refresh {
		// Dropping the cache is enough: ensureLoaded reconnects when a
		// push-only list is missing, and afterLogin then resends it.
		c.cache.statusPages.invalidate()
	}
	if err := c.ensureLoaded(ctx, c.cache.statusPages, nil); err != nil {
		return nil, err
	}
	return c.cache.statusPages.all(), nil
}

// DeleteStatusPage removes a page along with its groups and incidents.
func (c *Client) DeleteStatusPage(ctx context.Context, slug string) error {
	// Not routed through mutate: no push follows this, so there is nothing to
	// wait for. The cached list is dropped instead, so a later read reconnects.
	if err := c.call(ctx, nil, "deleteStatusPage", slug); err != nil {
		return err
	}
	c.cache.statusPages.invalidate()
	return nil
}

// PostIncident creates or updates the pinned incident of a status page.
//
// A page holds at most one pinned incident: posting always pins the result, so
// this doubles as "replace the current banner".
func (c *Client) PostIncident(ctx context.Context, slug string, incident StatusPageIncident) (*StatusPageIncident, error) {
	var resp struct {
		ackEnvelope
		Incident *StatusPageIncident `json:"incident"`
	}
	if err := c.call(ctx, &resp, "postIncident", slug, incident); err != nil {
		return nil, err
	}
	if resp.Incident == nil {
		return nil, fmt.Errorf("server accepted the incident but returned nothing")
	}
	return resp.Incident, nil
}

// EditIncident updates an incident without changing its pinned state.
func (c *Client) EditIncident(ctx context.Context, slug string, id int, incident StatusPageIncident) (*StatusPageIncident, error) {
	var resp struct {
		ackEnvelope
		Incident *StatusPageIncident `json:"incident"`
	}
	if err := c.call(ctx, &resp, "editIncident", slug, id, incident); err != nil {
		return nil, err
	}
	return resp.Incident, nil
}

// ResolveIncident marks an incident resolved, which also unpins it.
func (c *Client) ResolveIncident(ctx context.Context, slug string, id int) error {
	return c.call(ctx, nil, "resolveIncident", slug, id)
}

// DeleteIncident removes an incident outright.
func (c *Client) DeleteIncident(ctx context.Context, slug string, id int) error {
	return c.call(ctx, nil, "deleteIncident", slug, id)
}

// UnpinIncident hides the banner without deleting the incident.
func (c *Client) UnpinIncident(ctx context.Context, slug string) error {
	return c.call(ctx, nil, "unpinIncident", slug)
}

// GetIncidentHistory returns the incidents of a page, newest first.
func (c *Client) GetIncidentHistory(ctx context.Context, slug string) ([]StatusPageIncident, error) {
	var resp struct {
		ackEnvelope
		Incidents []StatusPageIncident `json:"incidents"`
	}
	// The cursor argument pages through the history; nil asks for the first page.
	if err := c.call(ctx, &resp, "getIncidentHistory", slug, nil); err != nil {
		return nil, err
	}
	return resp.Incidents, nil
}

// SetMaintenanceStatusPages sets which status pages a maintenance window shows
// on. Like the monitor association, this replaces the whole set.
func (c *Client) SetMaintenanceStatusPages(ctx context.Context, maintenanceID int, statusPageIDs []int) error {
	pages := make([]map[string]any, 0, len(statusPageIDs))
	for _, id := range statusPageIDs {
		pages = append(pages, map[string]any{"id": id})
	}
	return c.call(ctx, nil, "addMaintenanceStatusPage", maintenanceID, pages)
}

// GetMaintenanceStatusPages returns the status page IDs attached to a window.
func (c *Client) GetMaintenanceStatusPages(ctx context.Context, maintenanceID int) ([]int, error) {
	var resp struct {
		ackEnvelope
		StatusPages []struct {
			ID int `json:"id"`
		} `json:"statusPages"`
	}
	if err := c.call(ctx, &resp, "getMaintenanceStatusPage", maintenanceID); err != nil {
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
func normalizeStatusPage(page *StatusPage) {
	// updateDomainNameList bails out unless this is an array, leaving the
	// existing domains untouched instead of clearing them.
	if page.DomainNameList == nil {
		page.DomainNameList = []string{}
	}
	if page.Theme == "" {
		page.Theme = "auto"
	}
}
