package kuma

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
)

// Bool is a boolean that also accepts the numeric and string forms Uptime Kuma
// emits.
//
// This is not defensiveness for its own sake. Several payloads are produced by
// dumping database rows straight to JSON (redbean's `bean.export()`, used for
// proxies, and `APIKey.toPublicJSON`), and SQLite stores booleans as 0/1. A
// plain bool field fails to unmarshal those, and because the pushed lists are
// decoded in an event handler the failure is silent: the cache simply stays
// empty and every read reports the entity as missing.
//
// Marshalling always produces a real boolean, which is what the server expects
// on the way in.
type Bool bool

// UnmarshalJSON accepts true/false, 1/0 and "1"/"0"/"true"/"false".
func (b *Bool) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)

	// null leaves the value at its zero state; a nil pointer field stays nil.
	if bytes.Equal(data, []byte("null")) {
		return nil
	}

	var asBool bool
	if err := json.Unmarshal(data, &asBool); err == nil {
		*b = Bool(asBool)
		return nil
	}

	var asNumber float64
	if err := json.Unmarshal(data, &asNumber); err == nil {
		*b = asNumber != 0
		return nil
	}

	var asString string
	if err := json.Unmarshal(data, &asString); err == nil {
		parsed, err := strconv.ParseBool(asString)
		if err != nil {
			return fmt.Errorf("cannot read %q as a boolean", asString)
		}
		*b = Bool(parsed)
		return nil
	}

	return fmt.Errorf("cannot read %s as a boolean", string(data))
}

// MarshalJSON always writes a JSON boolean.
func (b Bool) MarshalJSON() ([]byte, error) {
	return json.Marshal(bool(b))
}

// Value returns the plain Go bool.
func (b *Bool) Value() bool {
	if b == nil {
		return false
	}
	return bool(*b)
}

// BoolPtr wraps a plain bool for use in the pointer-typed wire structs.
func BoolPtr(v bool) *Bool {
	b := Bool(v)
	return &b
}
