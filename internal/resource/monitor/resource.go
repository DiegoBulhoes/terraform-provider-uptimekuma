// Package monitor holds one resource per Uptime Kuma monitor type. The whole
// CRUD cycle lives in resource.go; each type file only declares its own
// attributes and how they map onto the wire payload.
package monitor

import (
	"context"
	"fmt"
	"strconv"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// Resource implements every monitor type. Behavior comes from the
// TypeDef it is built with.
type Resource struct {
	def    TypeDef
	client common.KumaClient
}

var (
	_ resource.Resource                = (*Resource)(nil)
	_ resource.ResourceWithConfigure   = (*Resource)(nil)
	_ resource.ResourceWithImportState = (*Resource)(nil)
)

// New returns a factory for the given monitor type.
func New(def TypeDef) func() resource.Resource {
	return func() resource.Resource {
		return &Resource{def: def}
	}
}

func (r *Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_" + r.def.TypeName
}

func (r *Resource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, err := common.ConfigureClient(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected provider data", err.Error())
		return
	}
	r.client = client
}

func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	model := r.def.NewModel()
	resp.Diagnostics.Append(req.Plan.Get(ctx, model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	wire := &kuma.Monitor{Type: r.def.WireType}
	model.Base().applyBase(ctx, wire, &resp.Diagnostics)
	model.ApplyTo(ctx, wire, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	var id int
	err := common.RetryRPC(ctx, 3, func() error {
		var err error
		id, err = r.client.CreateMonitor(ctx, *wire)
		return err
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to create monitor", err.Error())
		return
	}

	tflog.Debug(ctx, "Created Uptime Kuma monitor", map[string]any{"id": id, "type": r.def.WireType})

	// Tags are separate associations, so they can only be attached once the
	// monitor exists.
	for _, tag := range model.Base().tagAssociations(ctx, &resp.Diagnostics) {
		if err := r.client.AddMonitorTag(ctx, int(tag.TagID.ValueInt64()), id, tag.Value.ValueString()); err != nil {
			resp.Diagnostics.AddError("Unable to attach tag to monitor", err.Error())
			return
		}
	}
	// `add` starts the monitor unless active was false, but it is cheap to
	// confirm and keeps Create and Update on the same path.
	r.reconcileActive(ctx, id, model.Base().Active, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	// Read back so every Computed attribute holds what the server decided.
	if !r.readInto(ctx, id, model, &resp.Diagnostics) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	model := r.def.NewModel()
	resp.Diagnostics.Append(req.State.Get(ctx, model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, ok := common.ParseID(model.Base().ID, &resp.Diagnostics)
	if !ok {
		return
	}

	wire, err := r.client.GetMonitor(ctx, id)
	if err != nil {
		if kuma.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read monitor", err.Error())
		return
	}

	// Guard against reading a monitor that another resource type owns, which
	// would otherwise silently rewrite unrelated state.
	if wire.Type != r.def.WireType {
		resp.Diagnostics.AddError(
			"Monitor type mismatch",
			fmt.Sprintf("Monitor %d is of type %q, but this resource manages %q monitors.", id, wire.Type, r.def.WireType),
		)
		return
	}

	model.Base().readBase(wire, &resp.Diagnostics)
	model.ReadFrom(ctx, wire, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, model)...)
}

func (r *Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	plan := r.def.NewModel()
	resp.Diagnostics.Append(req.Plan.Get(ctx, plan)...)
	state := r.def.NewModel()
	resp.Diagnostics.Append(req.State.Get(ctx, state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, ok := common.ParseID(state.Base().ID, &resp.Diagnostics)
	if !ok {
		return
	}

	// The payload is built from the plan alone, not merged onto what the server
	// currently holds.
	//
	// editMonitor writes every field it is given, so merging would look safer —
	// but it makes removing an attribute impossible: a value the user deleted
	// from the config arrives as null, leaves the wire struct untouched, and the
	// server's old value gets written straight back. Building from the plan makes
	// the config authoritative, which is what Terraform users expect. The
	// trade-off is that fields this provider does not model (such as the
	// condition tree editable in the web UI) are reset on update.
	wire := &kuma.Monitor{ID: id, Type: r.def.WireType}
	plan.Base().applyBase(ctx, wire, &resp.Diagnostics)
	plan.ApplyTo(ctx, wire, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	err := common.RetryRPC(ctx, 3, func() error {
		return r.client.UpdateMonitor(ctx, *wire)
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to update monitor", err.Error())
		return
	}

	r.reconcileTags(ctx, id, state.Base(), plan.Base(), &resp.Diagnostics)
	r.reconcileActive(ctx, id, plan.Base().Active, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	if !r.readInto(ctx, id, plan, &resp.Diagnostics) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	model := r.def.NewModel()
	resp.Diagnostics.Append(req.State.Get(ctx, model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, ok := common.ParseID(model.Base().ID, &resp.Diagnostics)
	if !ok {
		return
	}

	// deleteChildren is false: children of a group are managed by their own
	// resources, and cascading would delete state Terraform still tracks.
	err := common.RetryRPC(ctx, 3, func() error {
		return r.client.DeleteMonitor(ctx, id, false)
	})
	if err != nil && !kuma.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete monitor", err.Error())
	}
}

func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if _, err := strconv.Atoi(req.ID); err != nil {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Expected the numeric monitor ID, got %q.", req.ID),
		)
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// readInto refreshes a model from the server.
func (r *Resource) readInto(ctx context.Context, id int, model Model, diags *diag.Diagnostics) bool {
	wire, err := r.client.GetMonitor(ctx, id)
	if err != nil {
		diags.AddError("Unable to read monitor back after saving", err.Error())
		return false
	}
	model.Base().readBase(wire, diags)
	model.ReadFrom(ctx, wire, diags)
	return !diags.HasError()
}
