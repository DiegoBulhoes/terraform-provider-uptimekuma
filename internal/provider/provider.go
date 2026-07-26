package provider

import (
	"context"
	"time"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	kumadatasource "github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/datasource"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	kumaresource "github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/resource"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"
)

var _ provider.Provider = (*UptimeKumaProvider)(nil)

type UptimeKumaProvider struct {
	Version string
}

type UptimeKumaProviderModel struct {
	Endpoint           types.String `tfsdk:"endpoint"`
	Username           types.String `tfsdk:"username"`
	Password           types.String `tfsdk:"password"`
	Token              types.String `tfsdk:"token"`
	Timeout            types.Int64  `tfsdk:"timeout"`
	MaxRetries         types.Int64  `tfsdk:"max_retries"`
	InsecureSkipVerify types.Bool   `tfsdk:"insecure_skip_verify"`
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &UptimeKumaProvider{
			Version: version,
		}
	}
}

func (p *UptimeKumaProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "uptimekuma"
	resp.Version = p.Version
}

func (p *UptimeKumaProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Terraform provider for managing Uptime Kuma monitors, tags, notifications, maintenance windows and infrastructure.",
		Attributes: map[string]schema.Attribute{
			"endpoint": schema.StringAttribute{
				Description: "Base URL of the Uptime Kuma instance, for example `https://kuma.example.com`. Can also be set with the UPTIME_KUMA_URL environment variable.",
				Optional:    true,
			},
			"username": schema.StringAttribute{
				Description: "Username of the Uptime Kuma account. Can also be set with the UPTIME_KUMA_USERNAME environment variable.",
				Optional:    true,
			},
			"password": schema.StringAttribute{
				Description: "Password of the Uptime Kuma account. Can also be set with the UPTIME_KUMA_PASSWORD environment variable.",
				Optional:    true,
				Sensitive:   true,
			},
			"token": schema.StringAttribute{
				Description: "Current two-factor authentication code, required only when the account has 2FA enabled. Can also be set with the UPTIME_KUMA_TOKEN environment variable. Note that a code is single-use, so 2FA accounts are a poor fit for automation.",
				Optional:    true,
				Sensitive:   true,
			},
			"timeout": schema.Int64Attribute{
				Description: "How long to wait, in seconds, for the server to acknowledge an operation. Default: 30.",
				Optional:    true,
				Validators: []validator.Int64{
					int64validator.AtLeast(1),
				},
			},
			"max_retries": schema.Int64Attribute{
				Description: "How many times to retry an operation that failed for a transient reason, such as a dropped connection. Default: 3.",
				Optional:    true,
				Validators: []validator.Int64{
					int64validator.AtLeast(0),
				},
			},
			"insecure_skip_verify": schema.BoolAttribute{
				Description: "Skip TLS certificate verification. Useful for self-hosted instances behind a self-signed certificate. Default: false.",
				Optional:    true,
			},
		},
	}
}

func (p *UptimeKumaProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var config UptimeKumaProviderModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	cfg := resolveConfig(config)
	validateConfig(cfg, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}

	tflog.Debug(ctx, "Connecting to Uptime Kuma", map[string]any{
		"endpoint": cfg.Endpoint,
		"username": cfg.Username,
	})

	// Shared, not New: Uptime Kuma allows only 20 logins per minute across the
	// whole server, and Terraform configures the provider once per command.
	client, err := kuma.Shared(ctx, cfg)
	if err != nil {
		resp.Diagnostics.AddError("Unable to connect to Uptime Kuma", err.Error())
		return
	}

	resp.DataSourceData = client
	resp.ResourceData = client
}

// resolveConfig fills each setting from the configuration, then the environment,
// then the default.
func resolveConfig(config UptimeKumaProviderModel) kuma.Config {
	return kuma.Config{
		Endpoint:           common.EnvOrDefault(config.Endpoint, "UPTIME_KUMA_URL", ""),
		Username:           common.EnvOrDefault(config.Username, "UPTIME_KUMA_USERNAME", ""),
		Password:           common.EnvOrDefault(config.Password, "UPTIME_KUMA_PASSWORD", ""),
		TOTPToken:          common.EnvOrDefault(config.Token, "UPTIME_KUMA_TOKEN", ""),
		Timeout:            time.Duration(common.EnvOrDefaultInt(config.Timeout, "", 30)) * time.Second,
		MaxRetries:         common.EnvOrDefaultInt(config.MaxRetries, "", 3),
		InsecureSkipVerify: common.EnvOrDefaultBool(config.InsecureSkipVerify, "", false),
	}
}

// validateConfig reports everything missing at once, so the user does not have to
// re-run to find the second problem.
func validateConfig(cfg kuma.Config, diags *diag.Diagnostics) {
	if cfg.Endpoint == "" {
		diags.AddError(
			"Missing Uptime Kuma endpoint",
			"Set the provider's `endpoint` attribute or the UPTIME_KUMA_URL environment variable.",
		)
	}
	// Uptime Kuma has no API-key authentication for its Socket.IO API — the
	// api_key entity only guards the Prometheus /metrics endpoint — so a
	// username and password are the only way in.
	if cfg.Username == "" || cfg.Password == "" {
		diags.AddError(
			"Missing Uptime Kuma credentials",
			"Set the provider's `username` and `password` attributes, or the UPTIME_KUMA_USERNAME and UPTIME_KUMA_PASSWORD environment variables. Uptime Kuma does not support API-key authentication for the API this provider uses.",
		)
	}
}

func (p *UptimeKumaProvider) Resources(_ context.Context) []func() resource.Resource {
	return kumaresource.All()
}

func (p *UptimeKumaProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return kumadatasource.All()
}
