package kuma

import (
	"context"
	"fmt"
)

// CreateTag creates a tag and returns its ID.
func (c *Client) CreateTag(ctx context.Context, tag Tag) (int, error) {
	var resp struct {
		ackEnvelope
		Tag *Tag `json:"tag"`
	}
	if err := c.call(ctx, &resp, "addTag", tag); err != nil {
		return 0, err
	}
	if resp.Tag == nil || resp.Tag.ID == 0 {
		return 0, fmt.Errorf("server accepted the tag but returned no ID")
	}
	return resp.Tag.ID, nil
}

// UpdateTag saves an existing tag.
func (c *Client) UpdateTag(ctx context.Context, tag Tag) error {
	if tag.ID == 0 {
		return fmt.Errorf("tag ID is required to update")
	}
	return c.call(ctx, nil, "editTag", tag)
}

// DeleteTag removes a tag and detaches it from every monitor.
func (c *Client) DeleteTag(ctx context.Context, id int) error {
	return c.call(ctx, nil, "deleteTag", id)
}

// ListTags returns every tag. Tags are global, not per-user, and getTags returns
// the whole list in the acknowledgement, so no cache is involved.
func (c *Client) ListTags(ctx context.Context) ([]Tag, error) {
	var resp struct {
		ackEnvelope
		Tags []Tag `json:"tags"`
	}
	if err := c.call(ctx, &resp, "getTags"); err != nil {
		return nil, err
	}
	return resp.Tags, nil
}

// GetTag returns one tag by ID, or ErrNotFound. There is no single-tag getter on
// the server, so this filters the list.
func (c *Client) GetTag(ctx context.Context, id int) (*Tag, error) {
	tags, err := c.ListTags(ctx)
	if err != nil {
		return nil, err
	}
	for _, tag := range tags {
		if tag.ID == id {
			return &tag, nil
		}
	}
	return nil, ErrNotFound
}
