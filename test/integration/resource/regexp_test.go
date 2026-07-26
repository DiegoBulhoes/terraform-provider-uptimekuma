//go:build integration

package resource_test

import "regexp"

// Composite ID of a status page incident: <slug>/<numeric id>.
var regexpIncidentID = regexp.MustCompile(`^[a-z0-9-]+/\d+$`)
