// Package notification exposes the notification channels for reading.
package notification

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// ListDataSource lists the notification channels.
type ListDataSource struct {
	client common.KumaClient
}

type ListModel struct {
	Notifications []ListEntry `tfsdk:"notifications"`
}

type ListEntry struct {
	ID        types.String `tfsdk:"id"`
	Name      types.String `tfsdk:"name"`
	Type      types.String `tfsdk:"type"`
	Active    types.Bool   `tfsdk:"active"`
	IsDefault types.Bool   `tfsdk:"is_default"`
}

var (
	_ datasource.DataSource              = (*ListDataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*ListDataSource)(nil)
)

func NewList() datasource.DataSource {
	return &ListDataSource{}
}

func (d *ListDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_notifications"
}

func (d *ListDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	client, err := common.ConfigureClient(req.ProviderData)
	if err != nil {
		resp.Diagnostics.AddError("Unexpected provider data", err.Error())
		return
	}
	d.client = client
}

func (d *ListDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Lists the notification channels of the authenticated user. Handy for attaching an existing " +
			"channel to monitors managed by Terraform.",
		Attributes: map[string]schema.Attribute{
			"notifications": schema.ListNestedAttribute{
				Description: "The notification channels, ordered by ID. Provider-specific settings are not exposed, " +
					"because they can contain credentials.",
				Computed: true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":         schema.StringAttribute{Computed: true, Description: "Numeric ID of the notification."},
						"name":       schema.StringAttribute{Computed: true, Description: "Name of the notification."},
						"type":       schema.StringAttribute{Computed: true, Description: "Notification provider identifier."},
						"active":     schema.BoolAttribute{Computed: true, Description: "Whether the notification is enabled."},
						"is_default": schema.BoolAttribute{Computed: true, Description: "Whether it is applied to new monitors by default."},
					},
				},
			},
		},
	}
}

func (d *ListDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	notifications, err := d.client.ListNotifications(ctx)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list notifications", err.Error())
		return
	}

	ids := make([]int, 0, len(notifications))
	for id := range notifications {
		ids = append(ids, id)
	}
	sort.Ints(ids)

	entries := make([]ListEntry, 0, len(ids))
	for _, id := range ids {
		notification := notifications[id]

		// The type is stored inside the JSON config rather than in a column.
		notificationType := ""
		if notification.Config != "" {
			var config map[string]any
			if json.Unmarshal([]byte(notification.Config), &config) == nil {
				if value, ok := config["type"].(string); ok {
					notificationType = value
				}
			}
		}

		entries = append(entries, ListEntry{
			ID:        types.StringValue(strconv.Itoa(notification.ID)),
			Name:      types.StringValue(notification.Name),
			Type:      types.StringValue(notificationType),
			Active:    types.BoolValue(bool(notification.Active)),
			IsDefault: types.BoolValue(bool(notification.IsDefault)),
		})
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &ListModel{Notifications: entries})...)
}
