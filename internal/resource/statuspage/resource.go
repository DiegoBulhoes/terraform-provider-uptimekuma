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
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
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

type Model struct {
	ID     types.String `tfsdk:"id"`
	PageID types.Int64  `tfsdk:"page_id"`
	Slug   types.String `tfsdk:"slug"`
	Title  types.String `tfsdk:"title"`

	Description types.String `tfsdk:"description"`
	Icon        types.String `tfsdk:"icon"`
	Theme       types.String `tfsdk:"theme"`
	FooterText  types.String `tfsdk:"footer_text"`
	CustomCSS   types.String `tfsdk:"custom_css"`
	RSSTitle    types.String `tfsdk:"rss_title"`

	AutoRefreshInterval types.Int64 `tfsdk:"auto_refresh_interval"`

	ShowTags              types.Bool `tfsdk:"show_tags"`
	ShowPoweredBy         types.Bool `tfsdk:"show_powered_by"`
	ShowCertificateExpiry types.Bool `tfsdk:"show_certificate_expiry"`
	ShowOnlyLastHeartbeat types.Bool `tfsdk:"show_only_last_heartbeat"`

	AnalyticsID        types.String `tfsdk:"analytics_id"`
	AnalyticsScriptURL types.String `tfsdk:"analytics_script_url"`
	AnalyticsType      types.String `tfsdk:"analytics_type"`

	DomainNameList types.List `tfsdk:"domain_names"`

	Published types.Bool `tfsdk:"published"`

	Groups []GroupModel `tfsdk:"group"`
}

// GroupModel is one section of the page. Declared as a block so the order in the
// configuration is the order on the page.
type GroupModel struct {
	Name     types.String   `tfsdk:"name"`
	Monitors []MonitorModel `tfsdk:"monitor"`
}

// MonitorModel is a monitor inside a group.
type MonitorModel struct {
	MonitorID types.Int64  `tfsdk:"monitor_id"`
	SendURL   types.Bool   `tfsdk:"send_url"`
	URL       types.String `tfsdk:"url"`
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

func (r *Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A public status page, with the groups of monitors shown on it.",
		MarkdownDescription: "A public status page, with the groups of monitors shown on it.\n\n" +
			"~> **Three settings cannot be managed here.** `published`, the search-engine-index flag and the page " +
			"password are read-only for any API client: the handler that saves a status page has those assignments " +
			"commented out upstream, so no event can change them. Set them in the web UI.\n\n" +
			"~> **Group order is the display order.** Uptime Kuma derives each group's weight from its position, and " +
			"the same goes for monitors inside a group, so `group` and `monitor` are ordered blocks rather than sets.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "The page's slug, which is how the API addresses it.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"page_id": schema.Int64Attribute{
				Description: "Numeric ID of the page. Use this for `uptimekuma_maintenance.status_page_ids`.",
				Computed:    true,
			},
			"slug": schema.StringAttribute{
				Description: "URL slug, so the page is served at `/status/<slug>`. Lowercased by the server.",
				Required:    true,
			},
			"title": schema.StringAttribute{
				Description: "Title shown at the top of the page.",
				Required:    true,
			},
			"description": schema.StringAttribute{
				Description: "Description shown under the title.",
				Optional:    true,
			},
			"icon": schema.StringAttribute{
				Description: "Logo for the page. Either a URL, or a `data:image/png;base64,...` value that the " +
					"server stores as a file. PNG is the only accepted upload format. Default: `/icon.svg`.",
				Optional: true,
				Computed: true,
			},
			"theme": schema.StringAttribute{
				Description: "Page theme: `auto`, `light` or `dark`. Default: `auto`.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.String{
					stringvalidator.OneOf("auto", "light", "dark"),
				},
			},
			"footer_text": schema.StringAttribute{
				Description: "Text shown at the bottom of the page.",
				Optional:    true,
			},
			"custom_css": schema.StringAttribute{
				Description: "CSS injected into the page.",
				Optional:    true,
			},
			"rss_title": schema.StringAttribute{
				Description: "Title of the page's RSS feed.",
				Optional:    true,
			},
			"auto_refresh_interval": schema.Int64Attribute{
				Description: "How often the page refreshes itself, in seconds. Default: 300.",
				Optional:    true,
				Computed:    true,
				Validators: []validator.Int64{
					int64validator.AtLeast(5),
				},
			},
			"show_tags": schema.BoolAttribute{
				Description: "Show each monitor's tags. Default: false.",
				Optional:    true,
				Computed:    true,
			},
			"show_powered_by": schema.BoolAttribute{
				Description: "Show the \"Powered by Uptime Kuma\" footer. Default: true.",
				Optional:    true,
				Computed:    true,
			},
			"show_certificate_expiry": schema.BoolAttribute{
				Description: "Show how long each certificate has left. Default: false.",
				Optional:    true,
				Computed:    true,
			},
			"show_only_last_heartbeat": schema.BoolAttribute{
				Description: "Show only the most recent heartbeat instead of the bar. Default: false.",
				Optional:    true,
				Computed:    true,
			},
			"analytics_id": schema.StringAttribute{
				Description: "Site or tag ID for the analytics provider.",
				Optional:    true,
			},
			"analytics_script_url": schema.StringAttribute{
				Description: "Script URL for self-hosted analytics.",
				Optional:    true,
			},
			"analytics_type": schema.StringAttribute{
				Description: "Analytics provider: `google`, `umami`, `plausible`, `matomo` or `rybbit`.",
				Optional:    true,
				Validators: []validator.String{
					stringvalidator.OneOf("google", "umami", "plausible", "matomo", "rybbit"),
				},
			},
			"domain_names": schema.ListAttribute{
				Description: "Custom domains that serve this page.",
				Optional:    true,
				ElementType: types.StringType,
			},
			"published": schema.BoolAttribute{
				Description: "Whether the page is published. Read-only: no API event can change it.",
				Computed:    true,
			},
		},
		Blocks: map[string]schema.Block{
			"group": schema.ListNestedBlock{
				Description: "A section of the page. Order matters: it is the display order, and a group left out " +
					"of the configuration is deleted.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Description: "Heading for the section.",
							Required:    true,
						},
					},
					Blocks: map[string]schema.Block{
						"monitor": schema.ListNestedBlock{
							Description: "A monitor shown in this group, in display order.",
							NestedObject: schema.NestedBlockObject{
								Attributes: map[string]schema.Attribute{
									"monitor_id": schema.Int64Attribute{
										Description: "ID of the monitor to show.",
										Required:    true,
									},
									"send_url": schema.BoolAttribute{
										Description: "Make the monitor's name a link. Default: false.",
										Optional:    true,
										Computed:    true,
									},
									"url": schema.StringAttribute{
										Description: "Link the page shows for this monitor. Leave it unset to use " +
											"the monitor's own URL, which the server then reports here. It is only " +
											"reported at all when `send_url` is true, so with `send_url = false` " +
											"Terraform cannot detect drift in it.",
										Optional: true,
										// Computed as well: with send_url on and no
										// override, the server answers with the
										// monitor's own URL rather than null.
										Computed: true,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// config turns the model into the API payload, leaving the icon out: that goes
// in saveStatusPage's separate imgDataUrl argument.
func (m *Model) config(ctx context.Context, diags *diag.Diagnostics) kuma.StatusPage {
	page := kuma.StatusPage{
		Slug:                  strings.ToLower(m.Slug.ValueString()),
		Title:                 m.Title.ValueString(),
		Description:           common.StringPtr(m.Description),
		Theme:                 m.Theme.ValueString(),
		AutoRefreshInterval:   common.IntPtr(m.AutoRefreshInterval),
		ShowTags:              common.BoolPtr(m.ShowTags),
		ShowPoweredBy:         common.BoolPtr(m.ShowPoweredBy),
		ShowCertificateExpiry: common.BoolPtr(m.ShowCertificateExpiry),
		ShowOnlyLastHeartbeat: common.BoolPtr(m.ShowOnlyLastHeartbeat),
		CustomCSS:             common.StringPtr(m.CustomCSS),
		FooterText:            common.StringPtr(m.FooterText),
		RSSTitle:              common.StringPtr(m.RSSTitle),
		AnalyticsID:           common.StringPtr(m.AnalyticsID),
		AnalyticsScriptURL:    common.StringPtr(m.AnalyticsScriptURL),
		AnalyticsType:         common.StringPtr(m.AnalyticsType),
		DomainNameList:        common.StringListToSlice(ctx, m.DomainNameList),
	}
	if page.DomainNameList == nil {
		page.DomainNameList = []string{}
	}
	_ = diags
	return page
}

// groups turns the configured blocks into the payload, in order.
func (m *Model) groups() []kuma.StatusPageGroup {
	groups := make([]kuma.StatusPageGroup, 0, len(m.Groups))
	for _, group := range m.Groups {
		monitors := make([]kuma.StatusPageMonitor, 0, len(group.Monitors))
		for _, monitor := range group.Monitors {
			monitors = append(monitors, kuma.StatusPageMonitor{
				ID:      int(monitor.MonitorID.ValueInt64()),
				SendURL: common.BoolPtr(monitor.SendURL),
				URL:     common.StringPtr(monitor.URL),
			})
		}
		groups = append(groups, kuma.StatusPageGroup{
			Name:        group.Name.ValueString(),
			MonitorList: monitors,
		})
	}
	return groups
}

// icon picks the value for the imgDataUrl argument.
func (m *Model) icon() string {
	if common.IsSet(m.Icon) {
		return m.Icon.ValueString()
	}
	return defaultIcon
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
