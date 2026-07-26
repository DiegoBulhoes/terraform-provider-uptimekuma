package monitor

import (
	"context"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Two things editMonitor will not do.
//
// It never writes the active column, and there is no replace-all event for tags,
// so both are reconciled with their own calls after the update.

// reconcileActive pauses or resumes the monitor to match the plan.
//
// editMonitor deliberately never writes the `active` column — it assigns every
// other field one by one and leaves that one alone — so sending active in the
// payload has no effect on an existing monitor. Pausing and resuming are their
// own events, and this is the only thing that actually moves the state.
func (r *Resource) reconcileActive(ctx context.Context, id int, desired types.Bool, diags *diag.Diagnostics) {
	if !common.IsSet(desired) {
		return
	}

	wire, err := r.client.GetMonitor(ctx, id)
	if err != nil {
		diags.AddError("Unable to read monitor state before pausing or resuming", err.Error())
		return
	}

	current := true
	if wire.Active != nil {
		current = bool(*wire.Active)
	}
	if current == desired.ValueBool() {
		return
	}

	if desired.ValueBool() {
		err = r.client.ResumeMonitor(ctx, id)
	} else {
		err = r.client.PauseMonitor(ctx, id)
	}
	if err != nil {
		diags.AddError("Unable to change the monitor's active state", err.Error())
	}
}

// reconcileTags brings the monitor's tag associations in line with the plan.
//
// Tag associations are their own events, and the delete event identifies an
// association by (tag, monitor, value) — so a changed value is a remove plus an
// add, not an update.
func (r *Resource) reconcileTags(ctx context.Context, monitorID int, state, plan *BaseModel, diags *diag.Diagnostics) {
	current := state.tagAssociations(ctx, diags)
	desired := plan.tagAssociations(ctx, diags)
	if diags.HasError() {
		return
	}

	type key struct {
		tagID int
		value string
	}
	currentSet := make(map[key]struct{}, len(current))
	for _, tag := range current {
		currentSet[key{int(tag.TagID.ValueInt64()), tag.Value.ValueString()}] = struct{}{}
	}
	desiredSet := make(map[key]struct{}, len(desired))
	for _, tag := range desired {
		desiredSet[key{int(tag.TagID.ValueInt64()), tag.Value.ValueString()}] = struct{}{}
	}

	for k := range currentSet {
		if _, keep := desiredSet[k]; keep {
			continue
		}
		if err := r.client.DeleteMonitorTag(ctx, k.tagID, monitorID, k.value); err != nil {
			diags.AddError("Unable to detach tag from monitor", err.Error())
			return
		}
	}
	for k := range desiredSet {
		if _, exists := currentSet[k]; exists {
			continue
		}
		if err := r.client.AddMonitorTag(ctx, k.tagID, monitorID, k.value); err != nil {
			diags.AddError("Unable to attach tag to monitor", err.Error())
			return
		}
	}
}
