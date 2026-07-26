// Package statuspageincident manages the incident banner pinned to a status
// page.
package statuspageincident

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// Resource manages a status page incident.
//
// A page shows at most one incident at a time: posting always pins the result,
// and pinning replaces whatever was pinned before. Declaring two of these for
// the same page means the second one wins.
type Resource struct {
	client common.KumaClient
}

type Model struct {
	ID      types.String `tfsdk:"id"`
	Slug    types.String `tfsdk:"status_page_slug"`
	Title   types.String `tfsdk:"title"`
	Content types.String `tfsdk:"content"`
	Style   types.String `tfsdk:"style"`

	Pinned      types.Bool   `tfsdk:"pinned"`
	Active      types.Bool   `tfsdk:"active"`
	CreatedDate types.String `tfsdk:"created_date"`
}

var (
	_ resource.Resource                = (*Resource)(nil)
	_ resource.ResourceWithConfigure   = (*Resource)(nil)
	_ resource.ResourceWithImportState = (*Resource)(nil)
)

func New() resource.Resource {
	return &Resource{}
}

func (r *Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_status_page_incident"
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

func (r *Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The incident banner pinned to the top of a status page.",
		MarkdownDescription: "The incident banner pinned to the top of a status page.\n\n" +
			"~> **A page shows one incident at a time.** Posting an incident pins it and unpins whatever was there " +
			"before, so declaring two of these for the same page leaves only the last one visible.\n\n" +
			"~> **Destroying deletes the incident.** To keep the record but hide the banner, set `pinned = false` " +
			"instead, which resolves it.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Identifier in the form `<slug>/<incident id>`.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"status_page_slug": schema.StringAttribute{
				Description: "Slug of the status page this incident belongs to.",
				Required:    true,
				PlanModifiers: []planmodifier.String{
					// Incidents belong to a page; moving one means creating it
					// somewhere else.
					stringplanmodifier.RequiresReplace(),
				},
			},
			"title": schema.StringAttribute{
				Description: "Headline of the banner.",
				Required:    true,
			},
			"content": schema.StringAttribute{
				Description: "Body of the banner. Markdown is rendered.",
				Required:    true,
			},
			"style": schema.StringAttribute{
				Description: "Color of the banner: `info`, `warning`, `danger`, `primary`, `light` or `dark`. " +
					"Default: `warning`.",
				Optional: true,
				Computed: true,
				Validators: []validator.String{
					stringvalidator.OneOf("info", "warning", "danger", "primary", "light", "dark"),
				},
			},
			"pinned": schema.BoolAttribute{
				Description: "Whether the banner is visible. Setting this to false resolves the incident, which " +
					"keeps it in the page's history. Default: true.",
				Optional: true,
				Computed: true,
			},
			"active": schema.BoolAttribute{
				Description: "Whether the incident is still open, as reported by the server.",
				Computed:    true,
			},
			"created_date": schema.StringAttribute{
				Description: "When the incident was created, as reported by the server.",
				Computed:    true,
			},
		},
	}
}

func (m *Model) wire() kuma.StatusPageIncident {
	style := m.Style.ValueString()
	if style == "" {
		style = "warning"
	}
	return kuma.StatusPageIncident{
		Title:   m.Title.ValueString(),
		Content: m.Content.ValueString(),
		Style:   style,
	}
}

func (m *Model) readInto(slug string, incident *kuma.StatusPageIncident) {
	m.ID = types.StringValue(slug + "/" + strconv.Itoa(incident.ID))
	m.Slug = types.StringValue(slug)
	m.Title = types.StringValue(incident.Title)
	m.Content = types.StringValue(incident.Content)
	m.Style = types.StringValue(incident.Style)
	m.Pinned = common.BoolOrFalse(incident.Pin)
	m.Active = common.BoolOrFalse(incident.Active)
	m.CreatedDate = types.StringValue(incident.CreatedDate)
}

// parseID splits the composite ID back into slug and incident ID.
func parseID(id string) (string, int, error) {
	slug, rest, found := strings.Cut(id, "/")
	if !found {
		return "", 0, fmt.Errorf("expected `<slug>/<incident id>`, got %q", id)
	}
	// An empty slug would reach the server as a request for the incidents of no
	// page at all, and the answer to that is an empty list — which Read would
	// interpret as the incident being gone and quietly drop it from state.
	if slug == "" {
		return "", 0, fmt.Errorf("expected a status page slug before the `/` in %q", id)
	}
	incidentID, err := strconv.Atoi(rest)
	if err != nil {
		return "", 0, fmt.Errorf("expected a numeric incident ID in %q", id)
	}
	return slug, incidentID, nil
}

func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	slug := model.Slug.ValueString()

	// Captured before readInto, which overwrites it with what the server says.
	// The server always pins on post, so comparing afterwards would never see
	// that the user asked for an unpinned incident.
	wantPinned := model.Pinned

	var incident *kuma.StatusPageIncident
	err := common.RetryRPC(ctx, 3, func() error {
		var err error
		incident, err = r.client.PostIncident(ctx, slug, model.wire())
		return err
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to post incident", err.Error())
		return
	}

	model.readInto(slug, incident)

	// Posting always pins, so an explicit `pinned = false` needs a follow-up
	// resolve.
	if common.IsSet(wantPinned) && !wantPinned.ValueBool() {
		if err := r.client.ResolveIncident(ctx, slug, incident.ID); err != nil {
			resp.Diagnostics.AddError("Unable to resolve the incident after posting it", err.Error())
			return
		}
		model.Pinned = types.BoolValue(false)
		model.Active = types.BoolValue(false)
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model Model
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	slug, incidentID, err := parseID(model.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid resource ID", err.Error())
		return
	}

	// There is no getter for a single incident, so the history is filtered.
	history, err := r.client.GetIncidentHistory(ctx, slug)
	if err != nil {
		if kuma.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read the status page's incidents", err.Error())
		return
	}

	for _, incident := range history {
		if incident.ID == incidentID {
			model.readInto(slug, &incident)
			resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
			return
		}
	}
	resp.State.RemoveResource(ctx)
}

func (r *Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	var state Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	slug, incidentID, err := parseID(state.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid resource ID", err.Error())
		return
	}

	// Captured before readInto overwrites it with the server's value.
	wantPinned := plan.Pinned

	if !r.editContent(ctx, slug, incidentID, &plan, state.ID, &resp.Diagnostics) {
		return
	}
	if !r.reconcilePinned(ctx, slug, incidentID, wantPinned, state.Pinned, &plan, &resp.Diagnostics) {
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// editContent writes the title, body and style, and reads the result back.
func (r *Resource) editContent(
	ctx context.Context,
	slug string,
	incidentID int,
	plan *Model,
	stateID types.String,
	diags *diag.Diagnostics,
) bool {
	var incident *kuma.StatusPageIncident
	err := common.RetryRPC(ctx, 3, func() error {
		var editErr error
		incident, editErr = r.client.EditIncident(ctx, slug, incidentID, plan.wire())
		return editErr
	})
	if err != nil {
		diags.AddError("Unable to update incident", err.Error())
		return false
	}

	if incident == nil {
		plan.ID = stateID
		return true
	}
	plan.readInto(slug, incident)
	return true
}

// reconcilePinned applies a change to `pinned`, which editIncident leaves alone.
//
// Unpinning is a resolve. Pinning again is a fresh post, because nothing
// unresolves an incident.
func (r *Resource) reconcilePinned(
	ctx context.Context,
	slug string,
	incidentID int,
	wantPinned, wasPinned types.Bool,
	plan *Model,
	diags *diag.Diagnostics,
) bool {
	if !common.IsSet(wantPinned) || wantPinned.ValueBool() == wasPinned.ValueBool() {
		return true
	}

	if !wantPinned.ValueBool() {
		if err := r.client.ResolveIncident(ctx, slug, incidentID); err != nil {
			diags.AddError("Unable to resolve incident", err.Error())
			return false
		}
		plan.Pinned = types.BoolValue(false)
		plan.Active = types.BoolValue(false)
		return true
	}

	reposted, err := r.client.PostIncident(ctx, slug, plan.wire())
	if err != nil {
		diags.AddError("Unable to pin the incident again", err.Error())
		return false
	}
	plan.readInto(slug, reposted)
	return true
}

func (r *Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model Model
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	slug, incidentID, err := parseID(model.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Invalid resource ID", err.Error())
		return
	}

	err = common.RetryRPC(ctx, 3, func() error {
		return r.client.DeleteIncident(ctx, slug, incidentID)
	})
	if err != nil && !kuma.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete incident", err.Error())
	}
}

func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if _, _, err := parseID(req.ID); err != nil {
		resp.Diagnostics.AddError("Invalid import ID", err.Error())
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
