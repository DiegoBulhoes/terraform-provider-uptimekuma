package api

import (
	"context"
	"fmt"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma/wire"
)

// CreateTag creates a tag and returns its ID.
func CreateTag(ctx context.Context, c Caller, tag wire.Tag) (int, error) {
	var resp struct {
		wire.AckEnvelope
		Tag *wire.Tag `json:"tag"`
	}
	if err := c.Call(ctx, &resp, "addTag", tag); err != nil {
		return 0, err
	}
	if resp.Tag == nil || resp.Tag.ID == 0 {
		return 0, fmt.Errorf("server accepted the tag but returned no ID")
	}
	return resp.Tag.ID, nil
}

// UpdateTag saves an existing tag.
func UpdateTag(ctx context.Context, c Caller, tag wire.Tag) error {
	if tag.ID == 0 {
		return fmt.Errorf("tag ID is required to update")
	}
	return c.Call(ctx, nil, "editTag", tag)
}

// DeleteTag removes a tag and detaches it from every monitor.
func DeleteTag(ctx context.Context, c Caller, id int) error {
	return c.Call(ctx, nil, "deleteTag", id)
}

// ListTags returns every tag. Tags are global, not per-user, and getTags returns
// the whole list in the acknowledgement, so no wire.Cache is involved.
func ListTags(ctx context.Context, c Caller) ([]wire.Tag, error) {
	var resp struct {
		wire.AckEnvelope
		Tags []wire.Tag `json:"tags"`
	}
	if err := c.Call(ctx, &resp, "getTags"); err != nil {
		return nil, err
	}
	return resp.Tags, nil
}

// GetTag returns one tag by ID, or wire.ErrNotFound. There is no single-tag getter on
// the server, so this filters the list.
func GetTag(ctx context.Context, c Caller, id int) (*wire.Tag, error) {
	tags, err := ListTags(ctx, c)
	if err != nil {
		return nil, err
	}
	for _, tag := range tags {
		if tag.ID == id {
			return &tag, nil
		}
	}
	return nil, wire.ErrNotFound
}
