package statuspage

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The schema, including the tree of groups shown on the page.

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
