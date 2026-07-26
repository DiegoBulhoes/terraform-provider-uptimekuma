// Package settings exposes the instance settings for reading.
package settings

import (
	"context"
	"encoding/json"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
)

type DataSource struct {
	client common.KumaClient
}

type Model struct {
	Settings jsontypes.Normalized `tfsdk:"settings"`
}

var (
	_ datasource.DataSource              = (*DataSource)(nil)
	_ datasource.DataSourceWithConfigure = (*DataSource)(nil)
)

func New() datasource.DataSource {
	return &DataSource{}
}

func (d *DataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_settings"
}

func (d *DataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *DataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "The instance's general settings, as a JSON object. Which keys exist depends on the Uptime Kuma " +
			"version, so this is returned untyped rather than as fixed attributes.",
		Attributes: map[string]schema.Attribute{
			"settings": schema.StringAttribute{
				Description: "Every setting the server reports, as a JSON object.",
				Computed:    true,
				CustomType:  jsontypes.NormalizedType{},
			},
		},
	}
}

func (d *DataSource) Read(ctx context.Context, _ datasource.ReadRequest, resp *datasource.ReadResponse) {
	var settings map[string]any
	err := common.RetryRPC(ctx, 3, func() error {
		var err error
		settings, err = d.client.GetSettings(ctx)
		return err
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to read settings", err.Error())
		return
	}

	encoded, err := json.Marshal(settings)
	if err != nil {
		resp.Diagnostics.AddError("Unable to encode settings", err.Error())
		return
	}

	resp.Diagnostics.Append(resp.State.Set(ctx, &Model{
		Settings: jsontypes.NewNormalizedValue(string(encoded)),
	})...)
}
