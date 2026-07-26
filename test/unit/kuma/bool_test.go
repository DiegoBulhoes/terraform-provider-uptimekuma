package kuma_test

import (
	"encoding/json"
	"testing"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
)

// TestBoolUnmarshal covers the numeric and string forms Uptime Kuma emits.
//
// This matters more than it looks: several payloads are database rows dumped
// straight to JSON, SQLite stores booleans as 0/1, and those payloads arrive
// through event handlers where a decode error is silent. A regression here shows
// up as entities that appear not to exist.
func TestBoolUnmarshal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		json    string
		want    bool
		wantErr bool
	}{
		{name: "true", json: `true`, want: true},
		{name: "false", json: `false`, want: false},
		{name: "one", json: `1`, want: true},
		{name: "zero", json: `0`, want: false},
		{name: "other number", json: `2`, want: true},
		{name: "string one", json: `"1"`, want: true},
		{name: "string zero", json: `"0"`, want: false},
		{name: "string true", json: `"true"`, want: true},
		{name: "string false", json: `"false"`, want: false},
		{name: "null leaves the zero value", json: `null`, want: false},
		{name: "unparseable string", json: `"maybe"`, wantErr: true},
		{name: "object", json: `{}`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var got kuma.Bool
			err := json.Unmarshal([]byte(tt.json), &got)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected an error for %s, got %v", tt.json, bool(got))
				}
				return
			}
			if err != nil {
				t.Fatalf("unmarshalling %s: %v", tt.json, err)
			}
			if bool(got) != tt.want {
				t.Errorf("got %v, want %v", bool(got), tt.want)
			}
		})
	}
}

// TestBoolMarshal checks writes always produce a real JSON boolean, whatever the
// server sent us.
func TestBoolMarshal(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		value kuma.Bool
		want  string
	}{
		{value: kuma.Bool(true), want: "true"},
		{value: kuma.Bool(false), want: "false"},
	} {
		encoded, err := json.Marshal(tt.value)
		if err != nil {
			t.Fatalf("marshalling: %v", err)
		}
		if string(encoded) != tt.want {
			t.Errorf("got %s, want %s", encoded, tt.want)
		}
	}
}

// TestBoolInStruct exercises the pointer form used throughout the wire structs,
// where nil has to stay distinguishable from false.
func TestBoolInStruct(t *testing.T) {
	t.Parallel()

	var payload struct {
		Active *kuma.Bool `json:"active"`
	}

	if err := json.Unmarshal([]byte(`{"active":1}`), &payload); err != nil {
		t.Fatalf("unmarshalling: %v", err)
	}
	if payload.Active == nil {
		t.Fatal("active should not be nil")
	}
	if !payload.Active.Value() {
		t.Error("active should be true")
	}

	payload.Active = nil
	if err := json.Unmarshal([]byte(`{}`), &payload); err != nil {
		t.Fatalf("unmarshalling: %v", err)
	}
	if payload.Active != nil {
		t.Error("an absent key must leave the pointer nil")
	}
	// A nil pointer reads as false rather than panicking.
	if payload.Active.Value() {
		t.Error("nil should read as false")
	}
}

// TestBoolPtr covers the helper used to build write payloads.
func TestBoolPtr(t *testing.T) {
	t.Parallel()

	if got := kuma.BoolPtr(true); got == nil || !got.Value() {
		t.Errorf("BoolPtr(true) = %v", got)
	}
	if got := kuma.BoolPtr(false); got == nil || got.Value() {
		t.Errorf("BoolPtr(false) = %v", got)
	}
}
