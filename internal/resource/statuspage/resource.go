// Package statuspage manages Uptime Kuma status pages, including the groups of
// monitors shown on them.
package statuspage

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// defaultIcon is what Uptime Kuma itself uses when no logo was uploaded.
const defaultIcon = "/icon.svg"

// Resource manages a status page.
//
// The Terraform ID is the slug, because every API event addresses a page that
// way. The numeric ID is exposed separately as `page_id`, which is what
// `uptimekuma_maintenance.status_page_ids` needs.
type Resource struct {
	client common.KumaClient
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
	resp.TypeName = req.ProviderTypeName + "_status_page"
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

// readInto fills the model from the server's configuration and group tree.
func (m *Model) readInto(page *kuma.StatusPage, groups []kuma.StatusPageGroup, diags *diag.Diagnostics) {
	m.ID = types.StringValue(page.Slug)
	m.PageID = types.Int64Value(int64(page.ID))
	m.Slug = types.StringValue(page.Slug)
	m.Title = types.StringValue(page.Title)
	m.Description = common.StringValue(page.Description)
	m.Theme = types.StringValue(page.Theme)
	m.FooterText = common.OptionalString(page.FooterText)
	m.CustomCSS = common.OptionalString(page.CustomCSS)
	m.RSSTitle = common.OptionalString(page.RSSTitle)
	m.AutoRefreshInterval = common.IntValue(page.AutoRefreshInterval)
	m.ShowTags = common.BoolOrFalse(page.ShowTags)
	m.ShowPoweredBy = common.BoolOrTrue(page.ShowPoweredBy)
	m.ShowCertificateExpiry = common.BoolOrFalse(page.ShowCertificateExpiry)
	m.ShowOnlyLastHeartbeat = common.BoolOrFalse(page.ShowOnlyLastHeartbeat)
	m.AnalyticsID = common.OptionalString(page.AnalyticsID)
	m.AnalyticsScriptURL = common.OptionalString(page.AnalyticsScriptURL)
	m.AnalyticsType = common.OptionalString(page.AnalyticsType)
	m.Published = common.BoolOrTrue(page.Published)
	m.Icon = common.OptionalString(page.Icon)

	if len(page.DomainNameList) > 0 {
		list, listDiags := types.ListValueFrom(context.Background(), types.StringType, page.DomainNameList)
		diags.Append(listDiags...)
		m.DomainNameList = list
	} else {
		m.DomainNameList = types.ListNull(types.StringType)
	}

	if len(groups) == 0 {
		m.Groups = nil
		return
	}
	read := make([]GroupModel, 0, len(groups))
	for _, group := range groups {
		monitors := make([]MonitorModel, 0, len(group.MonitorList))
		for _, monitor := range group.MonitorList {
			monitors = append(monitors, MonitorModel{
				MonitorID: types.Int64Value(int64(monitor.ID)),
				SendURL:   common.BoolOrFalse(monitor.SendURL),
				// Only reported when send_url is true; keeping it null otherwise
				// avoids inventing a value.
				URL: common.OptionalString(monitor.URL),
			})
		}
		read = append(read, GroupModel{
			Name:     types.StringValue(group.Name),
			Monitors: monitors,
		})
	}
	m.Groups = read
}

func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// Creation takes only a title and a slug; everything else needs the save
	// call that follows.
	var slug string
	err := common.RetryRPC(ctx, 3, func() error {
		var err error
		slug, err = r.client.CreateStatusPage(ctx, model.Title.ValueString(), model.Slug.ValueString())
		return err
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to create status page", err.Error())
		return
	}

	if err := r.save(ctx, slug, &model); err != nil {
		// The page exists but is only half-configured. Saying so beats leaving
		// the user to guess why the next plan wants to change everything.
		resp.Diagnostics.AddError(
			"Status page created but not configured",
			fmt.Sprintf("The page %q was created, but saving its configuration failed: %s\n\n"+
				"Run apply again to finish configuring it.", slug, err),
		)
		// Still recorded in state, so the retry updates instead of colliding on
		// a duplicate slug.
		model.ID = types.StringValue(slug)
		model.Slug = types.StringValue(slug)
		resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
		return
	}

	if !r.readInto(ctx, model.Slug.ValueString(), &model, &resp.Diagnostics) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *Resource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var model Model
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	page, err := r.client.GetStatusPage(ctx, model.ID.ValueString())
	if err != nil {
		if kuma.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read status page", err.Error())
		return
	}

	groups, err := r.client.GetStatusPageGroups(ctx, page.Slug)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read the status page's groups", err.Error())
		return
	}

	model.readInto(page, groups, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	var state Model
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	// The save call addresses the page by its current slug, while the payload
	// carries the new one — that is how a rename works.
	currentSlug := state.ID.ValueString()

	if err := r.save(ctx, currentSlug, &plan); err != nil {
		resp.Diagnostics.AddError("Unable to update status page", err.Error())
		return
	}

	if !r.readInto(ctx, strings.ToLower(plan.Slug.ValueString()), &plan, &resp.Diagnostics) {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model Model
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := common.RetryRPC(ctx, 3, func() error {
		return r.client.DeleteStatusPage(ctx, model.ID.ValueString())
	})
	if err != nil && !kuma.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete status page", err.Error())
	}
}

func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if _, err := strconv.Atoi(req.ID); err == nil {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			fmt.Sprintf("Status pages are imported by slug, not by numeric ID. Got %q.", req.ID),
		)
		return
	}
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// save writes the configuration and the group tree in one call.
func (r *Resource) save(ctx context.Context, currentSlug string, model *Model) error {
	config := model.config(ctx, nil)
	groups := model.groups()
	icon := model.icon()

	return common.RetryRPC(ctx, 3, func() error {
		_, err := r.client.SaveStatusPage(ctx, currentSlug, config, icon, groups)
		return err
	})
}

func (r *Resource) readInto(ctx context.Context, slug string, model *Model, diags *diag.Diagnostics) bool {
	page, err := r.client.GetStatusPage(ctx, slug)
	if err != nil {
		diags.AddError("Unable to read the status page back after saving", err.Error())
		return false
	}
	groups, err := r.client.GetStatusPageGroups(ctx, page.Slug)
	if err != nil {
		diags.AddError("Unable to read the status page's groups", err.Error())
		return false
	}
	model.readInto(page, groups, diags)
	return !diags.HasError()
}
