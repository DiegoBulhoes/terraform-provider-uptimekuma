package notification

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// managedNotificationKeys are the fields this resource exposes as first-class
// attributes. Uptime Kuma stores them inside the same JSON blob as the
// provider-specific settings, so they have to be filtered out of `settings` on
// read to avoid reporting a permanent diff.
var managedNotificationKeys = map[string]struct{}{
	"id":            {},
	"name":          {},
	"type":          {},
	"isDefault":     {},
	"applyExisting": {},
	"active":        {},
	"userId":        {},
}

// Resource manages a notification channel.
//
// Uptime Kuma has around a hundred notification providers and stores each
// channel as a single JSON document, with only `name` and `isDefault` promoted to
// columns. Rather than model every provider, this resource takes the
// provider-specific fields as a JSON object in `settings`, which covers all of
// them and keeps working when upstream adds more.
type Resource struct {
	client common.KumaClient
}

type Model struct {
	ID            types.String         `tfsdk:"id"`
	Name          types.String         `tfsdk:"name"`
	Type          types.String         `tfsdk:"type"`
	IsDefault     types.Bool           `tfsdk:"is_default"`
	ApplyExisting types.Bool           `tfsdk:"apply_existing"`
	Settings      jsontypes.Normalized `tfsdk:"settings"`
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
	resp.TypeName = req.ProviderTypeName + "_notification"
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
		Description: "A notification channel. The provider-specific options go in `settings` as a JSON object, " +
			"which is how Uptime Kuma stores them internally.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Numeric ID of the notification, assigned by Uptime Kuma.",
				Computed:    true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(),
				},
			},
			"name": schema.StringAttribute{
				Description: "Friendly name shown when picking notifications for a monitor.",
				Required:    true,
			},
			"type": schema.StringAttribute{
				Description: "Notification provider identifier, for example `webhook`, `slack`, `telegram` or `smtp`. " +
					"See the Uptime Kuma UI for the full list; the provider does not restrict the value so new " +
					"upstream providers work without a provider release.",
				Required: true,
			},
			"is_default": schema.BoolAttribute{
				Description: "Enable this notification by default on newly created monitors. Default: false.",
				Optional:    true,
				Computed:    true,
			},
			"apply_existing": schema.BoolAttribute{
				Description: "When true, attach this notification to every existing monitor at apply time. " +
					"This is a one-shot action on the server, not stored state, so Terraform cannot detect drift in it.",
				Optional: true,
			},
			"settings": schema.StringAttribute{
				Description: "Provider-specific options as a JSON object, for example " +
					"`jsonencode({ webhookURL = \"https://...\", webhookContentType = \"json\" })`. " +
					"Compared semantically, so formatting and key order do not cause diffs.",
				Optional:   true,
				Computed:   true,
				CustomType: jsontypes.NormalizedType{},
			},
		},
	}
}

// BuildPayload merges the provider-specific settings with the promoted
// attributes into the single flat object Uptime Kuma stores.
//
// Kept as a plain function so the merge and its validation can be tested without
// a Terraform plan or a server.
func BuildPayload(name, notificationType string, isDefault, applyExisting bool, settingsJSON string) (map[string]any, error) {
	settings := map[string]any{}
	if settingsJSON != "" {
		if err := json.Unmarshal([]byte(settingsJSON), &settings); err != nil {
			return nil, fmt.Errorf("`settings` must be a JSON object: %w", err)
		}
		// JSON null decodes into a nil map without complaint; say so rather than
		// quietly treating it as "no settings".
		if settings == nil {
			return nil, fmt.Errorf("`settings` must be a JSON object, not null")
		}
	}

	// Guard against a settings blob that tries to override the first-class
	// attributes, which would make state and server disagree.
	for key := range settings {
		if _, managed := managedNotificationKeys[key]; managed {
			return nil, fmt.Errorf("`settings` must not contain %q; use the dedicated attribute instead", key)
		}
	}

	payload := make(map[string]any, len(settings)+4)
	for key, value := range settings {
		payload[key] = value
	}
	payload["name"] = name
	payload["type"] = notificationType
	payload["isDefault"] = isDefault
	payload["applyExisting"] = applyExisting
	return payload, nil
}

// payload builds the flat object the server expects, reporting problems as
// Terraform diagnostics.
func (m *Model) payload(diags *diag.Diagnostics) map[string]any {
	settings := ""
	if common.IsSet(m.Settings) {
		settings = m.Settings.ValueString()
	}

	payload, err := BuildPayload(
		m.Name.ValueString(),
		m.Type.ValueString(),
		m.IsDefault.ValueBool(),
		m.ApplyExisting.ValueBool(),
		settings,
	)
	if err != nil {
		diags.AddError("Invalid notification settings", err.Error())
		return nil
	}
	return payload
}

// readInto fills the model from a notification as the server reports it.
func (m *Model) readInto(notification *kuma.Notification, diags *diag.Diagnostics) {
	m.ID = types.StringValue(strconv.Itoa(notification.ID))
	m.Name = types.StringValue(notification.Name)
	m.IsDefault = types.BoolValue(bool(notification.IsDefault))

	// The whole channel comes back as one JSON string. Split it into the
	// promoted attributes and the remainder, which is what `settings` holds.
	stored := map[string]any{}
	if notification.Config != "" {
		if err := json.Unmarshal([]byte(notification.Config), &stored); err != nil {
			diags.AddError(
				"Unable to read notification settings",
				fmt.Sprintf("The server returned a config that is not a JSON object: %s", err),
			)
			return
		}
	}

	if notificationType, ok := stored["type"].(string); ok {
		m.Type = types.StringValue(notificationType)
	}

	for key := range managedNotificationKeys {
		delete(stored, key)
	}

	if len(stored) == 0 {
		m.Settings = jsontypes.NewNormalizedValue("{}")
		return
	}

	encoded, err := json.Marshal(stored)
	if err != nil {
		diags.AddError("Unable to encode notification settings", err.Error())
		return
	}
	m.Settings = jsontypes.NewNormalizedValue(string(encoded))
}

func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	payload := model.payload(&resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	var id int
	err := common.RetryRPC(ctx, 3, func() error {
		var err error
		id, err = r.client.SaveNotification(ctx, nil, payload)
		return err
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to create notification", err.Error())
		return
	}

	notification, err := r.client.GetNotification(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read notification back after creating it", err.Error())
		return
	}

	applyExisting := model.ApplyExisting
	model.readInto(notification, &resp.Diagnostics)
	// applyExisting is a server-side action with no stored counterpart, so the
	// configured value is carried through rather than read back.
	model.ApplyExisting = applyExisting
	if resp.Diagnostics.HasError() {
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

	id, ok := common.ParseID(model.ID, &resp.Diagnostics)
	if !ok {
		return
	}

	notification, err := r.client.GetNotification(ctx, id)
	if err != nil {
		if kuma.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read notification", err.Error())
		return
	}

	applyExisting := model.ApplyExisting
	model.readInto(notification, &resp.Diagnostics)
	model.ApplyExisting = applyExisting
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *Resource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var model Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, ok := common.ParseID(model.ID, &resp.Diagnostics)
	if !ok {
		return
	}

	payload := model.payload(&resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	err := common.RetryRPC(ctx, 3, func() error {
		_, err := r.client.SaveNotification(ctx, &id, payload)
		return err
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to update notification", err.Error())
		return
	}

	notification, err := r.client.GetNotification(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read notification back after updating it", err.Error())
		return
	}

	applyExisting := model.ApplyExisting
	model.readInto(notification, &resp.Diagnostics)
	model.ApplyExisting = applyExisting
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *Resource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var model Model
	resp.Diagnostics.Append(req.State.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, ok := common.ParseID(model.ID, &resp.Diagnostics)
	if !ok {
		return
	}

	err := common.RetryRPC(ctx, 3, func() error {
		return r.client.DeleteNotification(ctx, id)
	})
	if err != nil && !kuma.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete notification", err.Error())
	}
}

func (r *Resource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
