package monitor

import (
	"context"
	"fmt"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// The schema shared by every monitor type. Each type merges its own attributes
// over this.

// Uptime Kuma applies no defaults of its own for these, and validate() rejects
// intervals below the minimum, so the provider supplies them.
const (
	defaultInterval       = 60
	defaultResendInterval = 0
	defaultMaxRetries     = 0
)

func (r *Resource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attributes := baseAttributes()
	for name, attribute := range r.def.Attributes {
		if _, clash := attributes[name]; clash {
			// A type-specific attribute overriding a base one is a programming
			// error, and silently winning would be worse than panicking here.
			panic(fmt.Sprintf("monitor type %q redefines base attribute %q", r.def.TypeName, name))
		}
		attributes[name] = attribute
	}

	resp.Schema = schema.Schema{
		Description: r.def.Description,
		Attributes:  attributes,
	}
}

// baseAttributes builds the shared part of the schema.
//
// Attributes the server fills in are Optional+Computed: Uptime Kuma normalizes
// several values (a missing retry interval becomes the check interval, for
// instance), and marking them Computed lets the provider store what the server
// actually kept instead of fighting it every plan.
func baseAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"id": schema.StringAttribute{
			Description: "Numeric ID of the monitor, assigned by Uptime Kuma.",
			Computed:    true,
			PlanModifiers: []planmodifier.String{
				stringplanmodifier.UseStateForUnknown(),
			},
		},
		"name": schema.StringAttribute{
			Description: "Friendly name shown in the Uptime Kuma dashboard.",
			Required:    true,
		},
		"description": schema.StringAttribute{
			Description: "Description of the monitor.",
			Optional:    true,
		},
		"active": schema.BoolAttribute{
			Description: "Whether the monitor is running. Set to false to pause it. Default: true.",
			Optional:    true,
			Computed:    true,
		},
		"interval": schema.Int64Attribute{
			Description: "Seconds between checks. Default: 60.",
			Optional:    true,
			Computed:    true,
			Validators: []validator.Int64{
				int64validator.AtLeast(kuma.MinIntervalSeconds),
			},
		},
		"retry_interval": schema.Int64Attribute{
			Description: "Seconds between retries after a failure. Defaults to the value of `interval`, matching the Uptime Kuma web UI.",
			Optional:    true,
			Computed:    true,
			Validators: []validator.Int64{
				int64validator.AtLeast(kuma.MinIntervalSeconds),
			},
		},
		"resend_interval": schema.Int64Attribute{
			Description: "Resend the notification every N checks while the monitor stays down. 0 disables resending. Default: 0.",
			Optional:    true,
			Computed:    true,
			Validators: []validator.Int64{
				int64validator.AtLeast(0),
			},
		},
		"max_retries": schema.Int64Attribute{
			Description: "How many times to retry before marking the monitor down. Default: 0.",
			Optional:    true,
			Computed:    true,
			Validators: []validator.Int64{
				int64validator.AtLeast(0),
			},
		},
		"timeout": schema.Float64Attribute{
			Description: "Request timeout in seconds.",
			Optional:    true,
			Computed:    true,
		},
		"upside_down": schema.BoolAttribute{
			Description: "Invert the result: a reachable service counts as down. Default: false.",
			Optional:    true,
			Computed:    true,
		},
		"weight": schema.Int64Attribute{
			Description: "Sort weight in the dashboard; higher sorts first.",
			Optional:    true,
			Computed:    true,
		},
		"parent_id": schema.Int64Attribute{
			Description: "ID of the parent group monitor. Use with `uptimekuma_monitor_group`.",
			Optional:    true,
		},
		"notification_ids": schema.SetAttribute{
			Description: "IDs of the notification channels to trigger for this monitor.",
			Optional:    true,
			ElementType: types.Int64Type,
		},
		"tags": schema.SetNestedAttribute{
			Description: "Tags attached to this monitor. Uptime Kuma stores these as separate associations, so the provider reconciles them after saving the monitor itself.",
			Optional:    true,
			NestedObject: schema.NestedAttributeObject{
				Attributes: map[string]schema.Attribute{
					"tag_id": schema.Int64Attribute{
						Description: "ID of the tag, from `uptimekuma_tag`.",
						Required:    true,
					},
					"value": schema.StringAttribute{
						Description: "Optional per-monitor value for the tag, for example an environment name.",
						Optional:    true,
					},
				},
			},
		},
	}
}
