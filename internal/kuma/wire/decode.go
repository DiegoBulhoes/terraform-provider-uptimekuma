package wire

import (
	"encoding/json"
	"strconv"
)

// AckEnvelope is the shape shared by nearly every Uptime Kuma acknowledgement.
//
// OK is a pointer because a handful of handlers answer with a bare value
// instead of an object — `needSetup` replies with a boolean — and those must not
// be treated as failures.
type AckEnvelope struct {
	OK  *bool  `json:"ok"`
	Msg string `json:"msg"`
}

// Decoding for the push handlers, which cannot return an error: a failure there
// leaves the Cache empty and every object looks like it does not exist.

func FirstRaw(in []any) (json.RawMessage, bool) {
	if len(in) == 0 {
		return nil, false
	}
	raw, ok := in[0].(json.RawMessage)
	return raw, ok
}

// DecodeKeyedList decodes an object keyed by stringified ID.
func DecodeKeyedList[T any](in []any) (map[int]T, bool) {
	raw, ok := FirstRaw(in)
	if !ok {
		return nil, false
	}
	var keyed map[string]T
	if err := json.Unmarshal(raw, &keyed); err != nil {
		return nil, false
	}
	out := make(map[int]T, len(keyed))
	for key, item := range keyed {
		id, err := strconv.Atoi(key)
		if err != nil {
			continue
		}
		out[id] = item
	}
	return out, true
}

// DecodeArrayList decodes an array, indexing it by each element's own ID.
func DecodeArrayList[T any](in []any, id func(T) int) (map[int]T, bool) {
	raw, ok := FirstRaw(in)
	if !ok {
		return nil, false
	}
	var list []T
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, false
	}
	out := make(map[int]T, len(list))
	for _, item := range list {
		out[id(item)] = item
	}
	return out, true
}

func DecodeInt(in []any) (int, bool) {
	raw, ok := FirstRaw(in)
	if !ok {
		return 0, false
	}
	var v int
	if err := json.Unmarshal(raw, &v); err != nil {
		return 0, false
	}
	return v, true
}
