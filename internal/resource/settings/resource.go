package settings

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

// refusedSettings are keys the provider will not write.
//
// disableAuth is refused because enabling it makes the server disconnect every
// client, this one included, leaving the apply in an unrecoverable state.
var refusedSettings = map[string]string{
	"disableAuth": "Disabling authentication makes Uptime Kuma disconnect every client, including this provider, so the apply cannot complete. Change it in the web UI if you really want it.",
}

// Resource manages the instance's general settings.
//
// Settings are a singleton key/value store: there is nothing to create or
// delete, only read and write. The resource therefore adopts whatever is already
// there on create, and on destroy it merely stops managing the values rather
// than trying to remove them.
type Resource struct {
	client common.KumaClient
}

type Model struct {
	ID       types.String         `tfsdk:"id"`
	Settings jsontypes.Normalized `tfsdk:"settings"`
	All      jsontypes.Normalized `tfsdk:"all"`
}

var (
	_ resource.Resource              = (*Resource)(nil)
	_ resource.ResourceWithConfigure = (*Resource)(nil)
)

func New() resource.Resource {
	return &Resource{}
}

func (r *Resource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_settings"
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
		Description: "General settings of the Uptime Kuma instance. This is a singleton: only one instance of this " +
			"resource makes sense, it adopts the existing settings on create, and destroying it stops management " +
			"without reverting anything.",
		MarkdownDescription: "General settings of the Uptime Kuma instance.\n\n" +
			"~> **This is a singleton resource.** Uptime Kuma has no notion of creating or deleting its settings. " +
			"Creating this resource adopts the current values, and destroying it only removes them from Terraform " +
			"state — the server keeps whatever was last written.\n\n" +
			"~> Only the keys listed in `settings` are managed. Any key the provider does not know about is left " +
			"untouched, and `all` exposes the full set the server reports.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description: "Always `settings`, since the resource is a singleton.",
				Computed:    true,
			},
			"settings": schema.StringAttribute{
				Description: "Settings to manage, as a JSON object, for example " +
					"`jsonencode({ keepDataPeriodDays = 180, checkUpdate = false })`. " +
					"Keys absent from this object are left as they are on the server.",
				Required:   true,
				CustomType: jsontypes.NormalizedType{},
			},
			"all": schema.StringAttribute{
				Description: "Every setting the server reports, as a JSON object. Useful for discovering which keys " +
					"a given Uptime Kuma version supports.",
				Computed:   true,
				CustomType: jsontypes.NormalizedType{},
			},
		},
	}
}

// ParseManaged decodes the settings document and rejects the keys the provider
// refuses to manage.
//
// Kept as a plain function so the validation can be tested without a Terraform
// plan or a server.
func ParseManaged(settingsJSON string) (map[string]any, error) {
	var settings map[string]any
	if err := json.Unmarshal([]byte(settingsJSON), &settings); err != nil {
		return nil, fmt.Errorf("`settings` must be a JSON object: %w", err)
	}
	// JSON null decodes into a nil map without complaint, which would silently
	// mean "manage nothing" instead of telling the user their document is wrong.
	if settings == nil {
		return nil, fmt.Errorf("`settings` must be a JSON object, not null")
	}
	for key, reason := range refusedSettings {
		if _, present := settings[key]; present {
			return nil, fmt.Errorf("refusing to manage the %q setting: %s", key, reason)
		}
	}
	return settings, nil
}

// desired parses the managed settings out of the model.
func (m *Model) desired(diags *diag.Diagnostics) map[string]any {
	settings, err := ParseManaged(m.Settings.ValueString())
	if err != nil {
		diags.AddError("Invalid settings", err.Error())
		return nil
	}
	return settings
}

// apply merges the managed keys into the server's current settings and writes
// them back.
//
// setSettings replaces the whole document, so the current values have to be read
// first — writing only the managed keys would wipe everything else.
func (r *Resource) apply(ctx context.Context, model *Model, diags *diag.Diagnostics) {
	desired := model.desired(diags)
	if diags.HasError() {
		return
	}

	current, err := r.client.GetSettings(ctx)
	if err != nil {
		diags.AddError("Unable to read current settings", err.Error())
		return
	}

	merged := make(map[string]any, len(current)+len(desired))
	for key, value := range current {
		merged[key] = value
	}
	for key, value := range desired {
		merged[key] = value
	}

	tflog.Debug(ctx, "Writing Uptime Kuma settings", map[string]any{"managed_keys": len(desired)})

	if err := common.RetryRPC(ctx, 3, func() error {
		return r.client.SetSettings(ctx, merged, "")
	}); err != nil {
		diags.AddError("Unable to write settings", err.Error())
		return
	}

	r.refresh(ctx, model, diags)
}

// refresh reads the server's settings into the model, narrowing `settings` to
// the managed keys so unrelated values never appear as drift.
func (r *Resource) refresh(ctx context.Context, model *Model, diags *diag.Diagnostics) {
	desired := model.desired(diags)
	if diags.HasError() {
		return
	}

	current, err := r.client.GetSettings(ctx)
	if err != nil {
		diags.AddError("Unable to read settings", err.Error())
		return
	}

	managed := make(map[string]any, len(desired))
	for key := range desired {
		if value, present := current[key]; present {
			managed[key] = value
		} else {
			// The server drops keys it does not recognize; keeping the
			// configured value avoids a permanent diff on a setting that this
			// Uptime Kuma version simply ignores.
			managed[key] = desired[key]
		}
	}

	encodedManaged, err := json.Marshal(managed)
	if err != nil {
		diags.AddError("Unable to encode settings", err.Error())
		return
	}
	encodedAll, err := json.Marshal(current)
	if err != nil {
		diags.AddError("Unable to encode the full settings document", err.Error())
		return
	}

	model.ID = types.StringValue("settings")
	model.Settings = jsontypes.NewNormalizedValue(string(encodedManaged))
	model.All = jsontypes.NewNormalizedValue(string(encodedAll))
}

func (r *Resource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var model Model
	resp.Diagnostics.Append(req.Plan.Get(ctx, &model)...)
	if resp.Diagnostics.HasError() {
		return
	}

	r.apply(ctx, &model, &resp.Diagnostics)
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

	r.refresh(ctx, &model, &resp.Diagnostics)
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

	r.apply(ctx, &model, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &model)...)
}

func (r *Resource) Delete(ctx context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	// Nothing to do: Uptime Kuma has no way to delete a setting, and reverting to
	// some notion of "default" would be a guess. Destroying the resource just
	// gives up management.
	resp.Diagnostics.AddWarning(
		"Settings were not reverted",
		"Uptime Kuma cannot delete settings, so the values written by this resource remain in effect. "+
			"The resource has only been removed from Terraform state.",
	)
}
