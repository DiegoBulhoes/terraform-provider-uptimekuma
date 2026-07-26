package monitor

import (
	"context"

	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/common"
	"github.com/DiegoBulhoes/terraform-provider-uptimekuma/internal/kuma"
	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// HTTPBase holds the attributes shared by the HTTP-based monitor types:
// http, keyword and json_query all issue the same request and differ only in how
// they judge the response.
type HTTPBase struct {
	URL                 types.String `tfsdk:"url"`
	Method              types.String `tfsdk:"method"`
	Body                types.String `tfsdk:"body"`
	Headers             types.String `tfsdk:"headers"`
	HTTPBodyEncoding    types.String `tfsdk:"http_body_encoding"`
	MaxRedirects        types.Int64  `tfsdk:"max_redirects"`
	AcceptedStatusCodes types.Set    `tfsdk:"accepted_status_codes"`
	IgnoreTLS           types.Bool   `tfsdk:"ignore_tls"`
	ExpiryNotification  types.Bool   `tfsdk:"expiry_notification"`
	DomainExpiryNotify  types.Bool   `tfsdk:"domain_expiry_notification"`
	ProxyID             types.Int64  `tfsdk:"proxy_id"`
	CacheBust           types.Bool   `tfsdk:"cache_bust"`

	AuthMethod        types.String `tfsdk:"auth_method"`
	BasicAuthUser     types.String `tfsdk:"basic_auth_user"`
	BasicAuthPass     types.String `tfsdk:"basic_auth_pass"`
	AuthDomain        types.String `tfsdk:"auth_domain"`
	AuthWorkstation   types.String `tfsdk:"auth_workstation"`
	BearerToken       types.String `tfsdk:"bearer_token"`
	OAuthClientID     types.String `tfsdk:"oauth_client_id"`
	OAuthClientSecret types.String `tfsdk:"oauth_client_secret"`
	OAuthTokenURL     types.String `tfsdk:"oauth_token_url"`
	OAuthScopes       types.String `tfsdk:"oauth_scopes"`
	OAuthAudience     types.String `tfsdk:"oauth_audience"`
	OAuthAuthMethod   types.String `tfsdk:"oauth_auth_method"`

	TLSCa   types.String `tfsdk:"tls_ca"`
	TLSCert types.String `tfsdk:"tls_cert"`
	TLSKey  types.String `tfsdk:"tls_key"`

	SaveResponse      types.Bool  `tfsdk:"save_response"`
	SaveErrorResponse types.Bool  `tfsdk:"save_error_response"`
	ResponseMaxLength types.Int64 `tfsdk:"response_max_length"`
}

// responseMaxLengthMax is the ceiling Monitor.validate enforces: 1 MiB
// (RESPONSE_BODY_LENGTH_MAX in src/util.ts).
const responseMaxLengthMax = 1024 * 1024

// httpAttributes returns the schema for HTTPBase.
func httpAttributes() map[string]schema.Attribute {
	return map[string]schema.Attribute{
		"url": schema.StringAttribute{
			Description: "URL to request.",
			Required:    true,
		},
		"method": schema.StringAttribute{
			Description: "HTTP method. Default: GET.",
			Optional:    true,
			Computed:    true,
			Validators: []validator.String{
				stringvalidator.OneOf("GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"),
			},
		},
		"body": schema.StringAttribute{
			Description: "Request body.",
			Optional:    true,
		},
		"headers": schema.StringAttribute{
			Description: "Additional request headers, as a JSON object string. Uptime Kuma validates that this parses as JSON.",
			Optional:    true,
		},
		"http_body_encoding": schema.StringAttribute{
			Description: "Encoding of the request body.",
			Optional:    true,
			Computed:    true,
			Validators: []validator.String{
				stringvalidator.OneOf("json", "form", "xml"),
			},
		},
		"max_redirects": schema.Int64Attribute{
			Description: "How many redirects to follow. Default: 10.",
			Optional:    true,
			Computed:    true,
			Validators: []validator.Int64{
				int64validator.AtLeast(0),
			},
		},
		"accepted_status_codes": schema.SetAttribute{
			Description: "Status codes counted as success. Ranges are allowed, for example `200-299`. Default: `[\"200-299\"]`.",
			Optional:    true,
			Computed:    true,
			ElementType: types.StringType,
		},
		"ignore_tls": schema.BoolAttribute{
			Description: "Ignore TLS/SSL certificate errors for this monitor. Default: false.",
			Optional:    true,
			Computed:    true,
		},
		"expiry_notification": schema.BoolAttribute{
			Description: "Notify before the TLS certificate expires. Default: true.",
			Optional:    true,
			Computed:    true,
		},
		"domain_expiry_notification": schema.BoolAttribute{
			Description: "Notify before the domain registration expires. Default: false.",
			Optional:    true,
			Computed:    true,
		},
		"proxy_id": schema.Int64Attribute{
			Description: "ID of the proxy to send the request through, from `uptimekuma_proxy`.",
			Optional:    true,
		},
		"cache_bust": schema.BoolAttribute{
			Description: "Append a cache-busting query parameter to each request. Default: false.",
			Optional:    true,
			Computed:    true,
		},
		"auth_method": schema.StringAttribute{
			Description: "Authentication method: `basic`, `bearer`, `ntlm`, `mtls` or `oauth2-cc`. Leave unset for no authentication.",
			Optional:    true,
			Validators: []validator.String{
				stringvalidator.OneOf("basic", "bearer", "ntlm", "mtls", "oauth2-cc"),
			},
		},
		"basic_auth_user": schema.StringAttribute{
			Description: "Username for `basic` and `ntlm` authentication.",
			Optional:    true,
		},
		"basic_auth_pass": schema.StringAttribute{
			Description: "Password for `basic` and `ntlm` authentication.",
			Optional:    true,
			Sensitive:   true,
		},
		"auth_domain": schema.StringAttribute{
			Description: "Domain for `ntlm` authentication.",
			Optional:    true,
		},
		"auth_workstation": schema.StringAttribute{
			Description: "Workstation for `ntlm` authentication.",
			Optional:    true,
		},
		"bearer_token": schema.StringAttribute{
			Description: "Token for `bearer` authentication.",
			Optional:    true,
			Sensitive:   true,
		},
		"oauth_client_id": schema.StringAttribute{
			Description: "Client ID for `oauth2-cc` authentication.",
			Optional:    true,
		},
		"oauth_client_secret": schema.StringAttribute{
			Description: "Client secret for `oauth2-cc` authentication.",
			Optional:    true,
			Sensitive:   true,
		},
		"oauth_token_url": schema.StringAttribute{
			Description: "Token endpoint for `oauth2-cc` authentication.",
			Optional:    true,
		},
		"oauth_scopes": schema.StringAttribute{
			Description: "Space-separated scopes for `oauth2-cc` authentication.",
			Optional:    true,
		},
		"oauth_audience": schema.StringAttribute{
			Description: "Audience for `oauth2-cc` authentication.",
			Optional:    true,
		},
		"oauth_auth_method": schema.StringAttribute{
			Description: "How to present the client credentials: `client_secret_basic` or `client_secret_post`.",
			Optional:    true,
			Validators: []validator.String{
				stringvalidator.OneOf("client_secret_basic", "client_secret_post"),
			},
		},
		"tls_ca": schema.StringAttribute{
			Description: "PEM-encoded CA certificate for `mtls` authentication.",
			Optional:    true,
		},
		"tls_cert": schema.StringAttribute{
			Description: "PEM-encoded client certificate for `mtls` authentication.",
			Optional:    true,
		},
		"tls_key": schema.StringAttribute{
			Description: "PEM-encoded client key for `mtls` authentication.",
			Optional:    true,
			Sensitive:   true,
		},
		"save_response": schema.BoolAttribute{
			Description: "Store the response body of successful checks, so notification templates can use " +
				"`heartbeatJSON.response`. Default: false.",
			Optional: true,
			Computed: true,
		},
		"save_error_response": schema.BoolAttribute{
			Description: "Store the response body of failed checks, so notification templates can use " +
				"`heartbeatJSON.response`. Default: true.",
			Optional: true,
			Computed: true,
		},
		"response_max_length": schema.Int64Attribute{
			Description: "How much of a stored response body to keep, in bytes. Default: 1024.",
			Optional:    true,
			Computed:    true,
			Validators: []validator.Int64{
				int64validator.Between(0, responseMaxLengthMax),
			},
		},
	}
}

func (m *HTTPBase) applyHTTPBase(ctx context.Context, wire *kuma.Monitor, diags *diag.Diagnostics) {
	wire.URL = common.StringPtr(m.URL)
	wire.Method = common.StringPtr(m.Method)
	wire.Body = common.StringPtr(m.Body)
	wire.Headers = common.StringPtr(m.Headers)
	wire.HTTPBodyEncoding = common.StringPtr(m.HTTPBodyEncoding)
	wire.MaxRedirects = common.IntPtr(m.MaxRedirects)
	wire.IgnoreTLS = common.BoolPtr(m.IgnoreTLS)
	wire.ExpiryNotification = common.BoolPtr(m.ExpiryNotification)
	wire.DomainExpiryNotify = common.BoolPtr(m.DomainExpiryNotify)
	wire.ProxyID = common.IntPtr(m.ProxyID)
	wire.CacheBust = common.BoolPtr(m.CacheBust)

	// Left nil, the client substitutes ["200-299"]; the server dereferences the
	// array unconditionally.
	if common.IsSet(m.AcceptedStatusCodes) {
		wire.AcceptedStatusCodes = common.StringSetToSlice(ctx, m.AcceptedStatusCodes)
	}

	wire.AuthMethod = common.StringPtr(m.AuthMethod)
	wire.BasicAuthUser = common.StringPtr(m.BasicAuthUser)
	wire.BasicAuthPass = common.StringPtr(m.BasicAuthPass)
	wire.AuthDomain = common.StringPtr(m.AuthDomain)
	wire.AuthWorkstation = common.StringPtr(m.AuthWorkstation)
	wire.BearerToken = common.StringPtr(m.BearerToken)
	wire.OAuthClientID = common.StringPtr(m.OAuthClientID)
	wire.OAuthClientSecret = common.StringPtr(m.OAuthClientSecret)
	wire.OAuthTokenURL = common.StringPtr(m.OAuthTokenURL)
	wire.OAuthScopes = common.StringPtr(m.OAuthScopes)
	wire.OAuthAudience = common.StringPtr(m.OAuthAudience)
	wire.OAuthAuthMethod = common.StringPtr(m.OAuthAuthMethod)

	wire.TLSCa = common.StringPtr(m.TLSCa)
	wire.TLSCert = common.StringPtr(m.TLSCert)
	wire.TLSKey = common.StringPtr(m.TLSKey)

	wire.SaveResponse = common.BoolPtr(m.SaveResponse)
	wire.SaveErrorResponse = common.BoolPtr(m.SaveErrorResponse)
	wire.ResponseMaxLength = common.IntPtr(m.ResponseMaxLength)

	_ = diags
}

func (m *HTTPBase) readHTTPBase(ctx context.Context, wire *kuma.Monitor, diags *diag.Diagnostics) {
	m.URL = common.StringValue(wire.URL)
	m.Method = common.StringValue(wire.Method)
	m.Body = common.StringValue(wire.Body)
	m.Headers = common.StringValue(wire.Headers)
	m.MaxRedirects = common.IntValue(wire.MaxRedirects)
	m.IgnoreTLS = common.BoolOrFalse(wire.IgnoreTLS)
	m.ExpiryNotification = common.BoolOrFalse(wire.ExpiryNotification)
	m.DomainExpiryNotify = common.BoolOrFalse(wire.DomainExpiryNotify)
	m.ProxyID = common.IntValue(wire.ProxyID)
	m.CacheBust = common.BoolOrFalse(wire.CacheBust)

	// The server stores an empty string when no encoding was chosen; Computed
	// attributes must never be left unknown, so fall back to the API default.
	if wire.HTTPBodyEncoding != nil && *wire.HTTPBodyEncoding != "" {
		m.HTTPBodyEncoding = types.StringValue(*wire.HTTPBodyEncoding)
	} else {
		m.HTTPBodyEncoding = types.StringValue("json")
	}

	if len(wire.AcceptedStatusCodes) > 0 {
		elements := make([]attr.Value, 0, len(wire.AcceptedStatusCodes))
		for _, code := range wire.AcceptedStatusCodes {
			elements = append(elements, types.StringValue(code))
		}
		set, setDiags := types.SetValue(types.StringType, elements)
		diags.Append(setDiags...)
		m.AcceptedStatusCodes = set
	} else {
		m.AcceptedStatusCodes = types.SetNull(types.StringType)
	}

	m.AuthMethod = common.OptionalString(wire.AuthMethod)
	m.BasicAuthUser = common.StringValue(wire.BasicAuthUser)
	m.BasicAuthPass = common.StringValue(wire.BasicAuthPass)
	m.AuthDomain = common.StringValue(wire.AuthDomain)
	m.AuthWorkstation = common.StringValue(wire.AuthWorkstation)
	m.BearerToken = common.StringValue(wire.BearerToken)
	m.OAuthClientID = common.StringValue(wire.OAuthClientID)
	m.OAuthClientSecret = common.StringValue(wire.OAuthClientSecret)
	m.OAuthTokenURL = common.StringValue(wire.OAuthTokenURL)
	m.OAuthScopes = common.StringValue(wire.OAuthScopes)
	m.OAuthAudience = common.StringValue(wire.OAuthAudience)
	m.OAuthAuthMethod = common.StringValue(wire.OAuthAuthMethod)

	m.TLSCa = common.StringValue(wire.TLSCa)
	m.TLSCert = common.StringValue(wire.TLSCert)
	m.TLSKey = common.StringValue(wire.TLSKey)

	m.SaveResponse = common.BoolOrFalse(wire.SaveResponse)
	// The server's default is true here, unlike the other two.
	m.SaveErrorResponse = common.BoolOrTrue(wire.SaveErrorResponse)
	m.ResponseMaxLength = common.IntValue(wire.ResponseMaxLength)

	_ = ctx
}
