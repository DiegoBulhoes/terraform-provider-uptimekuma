//go:build integration

package datasource_test

import "regexp"

// Error patterns the data sources are expected to produce. Kept in one place so
// the message wording lives next to the tests that depend on it.
var (
	regexpMissingLookup   = regexp.MustCompile(`Missing monitor lookup`)
	regexpAmbiguousLookup = regexp.MustCompile(`Ambiguous monitor lookup`)
	regexpNotFound        = regexp.MustCompile(`no monitor named`)
)
