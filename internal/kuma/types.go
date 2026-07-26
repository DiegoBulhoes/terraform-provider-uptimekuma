package kuma

import "encoding/json"

// The JSON tags in this file mirror the Uptime Kuma wire format exactly, and
// that format mixes naming conventions inside a single payload: `retryInterval`
// sits next to `accepted_statuscodes`, `mqttTopic` next to `basic_auth_user`.
// Do not "normalize" them — the names come from `server/model/monitor.js`
// (toJSON) and `db/knex_init_db.js`, and the server matches them literally.
//
// Reads and writes share the same shape. The `add`/`editMonitor` handlers call
// redbean's `bean.import()`, which maps the very keys toJSON emits back onto
// database columns, so a struct round-trips without a separate write model.
//
// Pointers are used for optional fields so that "unset" and "zero" stay
// distinguishable, which matters for Terraform's null semantics.
//
// Optional fields also carry omitempty, and that is a compatibility
// requirement rather than a stylistic choice. On create the server feeds the
// payload to `bean.import()`, which turns each key into a column in the INSERT —
// so sending `"bearer_token": null` to an Uptime Kuma that predates that column
// fails the whole statement with "table monitor has no column named
// bearer_token". Omitting absent fields keeps one payload working across 2.2 to
// 2.4.
//
// Omitting a field does not prevent clearing it: editMonitor assigns every
// column explicitly from the payload, so a missing key arrives as undefined and
// the column ends up NULL. The exceptions are marked individually below.

// Monitor is a single check. Which fields apply depends on Type; see the
// per-type resources in internal/resource.
type Monitor struct {
	ID          int     `json:"id,omitempty"`
	Name        string  `json:"name"`
	Type        string  `json:"type"`
	Description *string `json:"description,omitempty"`
	Active      *Bool   `json:"active,omitempty"`

	// Scheduling.
	Interval       int      `json:"interval"`
	RetryInterval  int      `json:"retryInterval"`
	ResendInterval int      `json:"resendInterval"`
	MaxRetries     int      `json:"maxretries"`
	Timeout        *float64 `json:"timeout,omitempty"`

	// Presentation and hierarchy.
	Weight     *int  `json:"weight,omitempty"`
	Parent     *int  `json:"parent,omitempty"`
	UpsideDown *Bool `json:"upsideDown,omitempty"`

	// Relations. NotificationIDList is a set encoded as an object
	// ({"1": true}), not an array.
	//
	// Deliberately without omitempty: an empty map is how notifications get
	// detached, and omitting the key leaves the existing links in place.
	NotificationIDList map[string]bool `json:"notificationIDList"`
	Tags               []MonitorTag    `json:"tags,omitempty"`

	// HTTP family (http, keyword, json-query, real-browser).
	URL                 *string  `json:"url,omitempty"`
	Method              *string  `json:"method,omitempty"`
	Body                *string  `json:"body,omitempty"`
	Headers             *string  `json:"headers,omitempty"`
	HTTPBodyEncoding    *string  `json:"httpBodyEncoding,omitempty"`
	MaxRedirects        *int     `json:"maxredirects,omitempty"`
	AcceptedStatusCodes []string `json:"accepted_statuscodes"`
	IgnoreTLS           *Bool    `json:"ignoreTls,omitempty"`
	ExpiryNotification  *Bool    `json:"expiryNotification,omitempty"`
	DomainExpiryNotify  *Bool    `json:"domainExpiryNotification,omitempty"`
	ProxyID             *int     `json:"proxyId,omitempty"`
	CacheBust           *Bool    `json:"cacheBust,omitempty"`

	// Authentication for the HTTP family.
	AuthMethod        *string `json:"authMethod,omitempty"`
	BasicAuthUser     *string `json:"basic_auth_user,omitempty"`
	BasicAuthPass     *string `json:"basic_auth_pass,omitempty"`
	AuthDomain        *string `json:"authDomain,omitempty"`
	AuthWorkstation   *string `json:"authWorkstation,omitempty"`
	BearerToken       *string `json:"bearer_token,omitempty"`
	OAuthClientID     *string `json:"oauth_client_id,omitempty"`
	OAuthClientSecret *string `json:"oauth_client_secret,omitempty"`
	OAuthTokenURL     *string `json:"oauth_token_url,omitempty"`
	OAuthScopes       *string `json:"oauth_scopes,omitempty"`
	OAuthAudience     *string `json:"oauth_audience,omitempty"`
	OAuthAuthMethod   *string `json:"oauth_auth_method,omitempty"`
	TLSCa             *string `json:"tlsCa,omitempty"`
	TLSCert           *string `json:"tlsCert,omitempty"`
	TLSKey            *string `json:"tlsKey,omitempty"`

	// Keyword and JSON query.
	Keyword                     *string `json:"keyword,omitempty"`
	InvertKeyword               *Bool   `json:"invertKeyword,omitempty"`
	JSONPath                    *string `json:"jsonPath,omitempty"`
	JSONPathOperator            *string `json:"jsonPathOperator,omitempty"`
	ExpectedValue               *string `json:"expectedValue,omitempty"`
	RetryOnlyOnStatusCodeFailed *Bool   `json:"retryOnlyOnStatusCodeFailure,omitempty"`

	// Host-based checks (port, ping, dns, snmp, smtp, radius…).
	Hostname *string `json:"hostname,omitempty"`
	Port     *int    `json:"port,omitempty"`
	IPFamily *string `json:"ipFamily,omitempty"`

	// Ping.
	PacketSize            *int  `json:"packetSize,omitempty"`
	PingNumeric           *Bool `json:"ping_numeric,omitempty"`
	PingCount             *int  `json:"ping_count,omitempty"`
	PingPerRequestTimeout *int  `json:"ping_per_request_timeout,omitempty"`

	// DNS.
	DNSResolveServer *string `json:"dns_resolve_server,omitempty"`
	DNSResolveType   *string `json:"dns_resolve_type,omitempty"`
	DNSLastResult    *string `json:"dns_last_result,omitempty"`

	// Push.
	PushToken *string `json:"pushToken,omitempty"`

	// Docker.
	DockerContainer *string `json:"docker_container,omitempty"`
	DockerHost      *int    `json:"docker_host,omitempty"`

	// Databases (postgres, mysql, sqlserver, mongodb, redis, oracledb).
	DatabaseConnectionString *string `json:"databaseConnectionString,omitempty"`
	DatabaseQuery            *string `json:"databaseQuery,omitempty"`

	// MQTT.
	MqttTopic          *string `json:"mqttTopic,omitempty"`
	MqttSuccessMessage *string `json:"mqttSuccessMessage,omitempty"`
	MqttCheckType      *string `json:"mqttCheckType,omitempty"`
	MqttUsername       *string `json:"mqttUsername,omitempty"`
	MqttPassword       *string `json:"mqttPassword,omitempty"`
	MqttWebsocketPath  *string `json:"mqttWebsocketPath,omitempty"`

	// gRPC.
	GrpcURL         *string `json:"grpcUrl,omitempty"`
	GrpcProtobuf    *string `json:"grpcProtobuf,omitempty"`
	GrpcBody        *string `json:"grpcBody,omitempty"`
	GrpcMetadata    *string `json:"grpcMetadata,omitempty"`
	GrpcMethod      *string `json:"grpcMethod,omitempty"`
	GrpcServiceName *string `json:"grpcServiceName,omitempty"`
	GrpcEnableTLS   *Bool   `json:"grpcEnableTls,omitempty"`

	// Kafka producer. Brokers and SASL options are arrays/objects on the wire
	// even though the database stores them as serialized JSON.
	KafkaProducerTopic       *string         `json:"kafkaProducerTopic,omitempty"`
	KafkaProducerBrokers     []string        `json:"kafkaProducerBrokers,omitempty"`
	KafkaProducerSsl         *Bool           `json:"kafkaProducerSsl,omitempty"`
	KafkaProducerAllowTopic  *Bool           `json:"kafkaProducerAllowAutoTopicCreation,omitempty"`
	KafkaProducerMessage     *string         `json:"kafkaProducerMessage,omitempty"`
	KafkaProducerSaslOptions json.RawMessage `json:"kafkaProducerSaslOptions,omitempty"`

	// RabbitMQ.
	RabbitmqNodes    []string `json:"rabbitmqNodes,omitempty"`
	RabbitmqUsername *string  `json:"rabbitmqUsername,omitempty"`
	RabbitmqPassword *string  `json:"rabbitmqPassword,omitempty"`

	// Radius. Note that SNMP v1/v2c reuses RadiusPassword as its community
	// string (server/monitor-types/snmp.js:34).
	RadiusUsername         *string `json:"radiusUsername,omitempty"`
	RadiusPassword         *string `json:"radiusPassword,omitempty"`
	RadiusSecret           *string `json:"radiusSecret,omitempty"`
	RadiusCalledStationID  *string `json:"radiusCalledStationId,omitempty"`
	RadiusCallingStationID *string `json:"radiusCallingStationId,omitempty"`

	// SNMP.
	SnmpOid         *string `json:"snmpOid,omitempty"`
	SnmpVersion     *string `json:"snmpVersion,omitempty"`
	SnmpV3Username  *string `json:"snmp_v3_username,omitempty"`
	ExpectedTLSAler *string `json:"expectedTlsAlert,omitempty"`

	// SMTP.
	SmtpSecurity *string `json:"smtpSecurity,omitempty"`

	// NTP.
	NtpStratumThreshold        *int     `json:"ntpStratumThreshold,omitempty"`
	NtpTimeOffsetThreshold     *float64 `json:"ntpTimeOffsetThreshold,omitempty"`
	NtpRootDispersionThreshold *float64 `json:"ntpRootDispersionThreshold,omitempty"`

	// Games.
	Game                 *string `json:"game,omitempty"`
	GamedigGivenPortOnly *Bool   `json:"gamedigGivenPortOnly,omitempty"`
	GamedigToken         *string `json:"gamedigToken,omitempty"`

	// Real browser.
	RemoteBrowser   *int `json:"remote_browser,omitempty"`
	ScreenshotDelay *int `json:"screenshot_delay,omitempty"`

	// Websocket upgrade.
	WsSubprotocol             *string `json:"wsSubprotocol,omitempty"`
	WsIgnoreSecWebsocketAccep *Bool   `json:"wsIgnoreSecWebsocketAcceptHeader,omitempty"`

	// System service and globalping.
	SystemServiceName *string `json:"system_service_name,omitempty"`
	Subtype           *string `json:"subtype,omitempty"`
	Location          *string `json:"location,omitempty"`
	Protocol          *string `json:"protocol,omitempty"`

	// Response saving.
	SaveResponse      *Bool `json:"saveResponse,omitempty"`
	SaveErrorResponse *Bool `json:"saveErrorResponse,omitempty"`
	ResponseMaxLength *int  `json:"responseMaxLength,omitempty"`

	// Conditions is the per-type condition tree the UI builds. Kept opaque so
	// the provider does not have to model every operator.
	Conditions json.RawMessage `json:"conditions,omitempty"`

	// Read-only fields the server computes. Never sent on write.
	ChildrenIDs   []int  `json:"childrenIDs,omitempty"`
	ForceInactive *Bool  `json:"forceInactive,omitempty"`
	PathName      string `json:"pathName,omitempty"`
}

// MonitorTag is a tag applied to a monitor, with the optional per-monitor value.
type MonitorTag struct {
	ID        int     `json:"id,omitempty"`
	TagID     int     `json:"tag_id"`
	MonitorID int     `json:"monitor_id,omitempty"`
	Value     *string `json:"value"`
	Name      string  `json:"name,omitempty"`
	Color     string  `json:"color,omitempty"`
}

// Tag is a label that can be attached to monitors.
type Tag struct {
	ID    int    `json:"id,omitempty"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

// Notification is a notification channel.
//
// Only `name` and `isDefault` become columns; the server stores the entire
// object as a JSON string in `notification.config`, and the provider type lives
// inside that JSON (server/notification.js, Notification.save). That is why
// Config carries the type-specific settings verbatim.
type Notification struct {
	ID        int    `json:"id,omitempty"`
	Name      string `json:"name"`
	Active    Bool   `json:"active,omitempty"`
	IsDefault Bool   `json:"isDefault,omitempty"`
	UserID    int    `json:"userId,omitempty"`

	// Config is the raw JSON string the server keeps. On the pushed list it
	// arrives as a string containing JSON, not as a nested object.
	Config string `json:"config,omitempty"`
}

// NotificationRequest is the payload for addNotification. The server flattens
// everything into one object, so the type-specific fields sit at the top level
// alongside name and type.
type NotificationRequest struct {
	Name          string `json:"name"`
	Type          string `json:"type"`
	IsDefault     Bool   `json:"isDefault"`
	ApplyExisting Bool   `json:"applyExisting"`

	// Settings holds the provider-specific keys, merged into the same object
	// before sending.
	Settings map[string]any `json:"-"`
}

// Maintenance is a maintenance window.
//
// Which scheduling fields the server reads depends on Strategy
// (server/model/maintenance.js, jsonToBean):
//   - "manual": none
//   - "single": dateRange only
//   - "cron": cron + durationMinutes
//   - "recurring-*": timeRange, weekdays, daysOfMonth, intervalDay
//
// DateRange is always dereferenced by the server, so it must be present even
// for strategies that ignore it.
type Maintenance struct {
	ID          int    `json:"id,omitempty"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Strategy    string `json:"strategy"`
	Active      *Bool  `json:"active,omitempty"`

	// DateRange is [start, end]; either entry may be null.
	DateRange []*string `json:"dateRange"`
	// TimeRange is [start, end] as wall-clock times.
	TimeRange   []TimePart `json:"timeRange,omitempty"`
	Weekdays    []int      `json:"weekdays,omitempty"`
	DaysOfMonth []any      `json:"daysOfMonth,omitempty"`
	IntervalDay *int       `json:"intervalDay,omitempty"`
	Cron        *string    `json:"cron,omitempty"`

	// DurationMinutes is what writes use; Duration is what reads return, in
	// seconds. The server converts between them.
	DurationMinutes *int `json:"durationMinutes,omitempty"`
	Duration        *int `json:"duration,omitempty"`

	// TimezoneOption is the write field and accepts "SAME_AS_SERVER"; Timezone
	// is the resolved value the server reports back.
	TimezoneOption *string `json:"timezoneOption"`
	Timezone       *string `json:"timezone,omitempty"`

	// Read-only status the server derives from the schedule.
	Status string `json:"status,omitempty"`
}

// TimePart is one end of a maintenance time range.
type TimePart struct {
	Hours   int `json:"hours"`
	Minutes int `json:"minutes"`
	Seconds int `json:"seconds,omitempty"`
}

// Proxy is an outbound proxy usable by HTTP monitors.
type Proxy struct {
	ID            int     `json:"id,omitempty"`
	Protocol      string  `json:"protocol"`
	Host          string  `json:"host"`
	Port          int     `json:"port"`
	Auth          Bool    `json:"auth"`
	Username      *string `json:"username"`
	Password      *string `json:"password"`
	Active        Bool    `json:"active"`
	Default       Bool    `json:"default"`
	ApplyExisting Bool    `json:"applyExisting,omitempty"`
}

// DockerHost is a Docker daemon that docker monitors can query.
type DockerHost struct {
	ID           int    `json:"id,omitempty"`
	Name         string `json:"name"`
	DockerType   string `json:"dockerType"`
	DockerDaemon string `json:"dockerDaemon"`
}

// RemoteBrowser is an external browser endpoint for real-browser monitors.
type RemoteBrowser struct {
	ID   int    `json:"id,omitempty"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

// APIKey grants access to the Prometheus /metrics endpoint. It is not a
// credential for the Socket.IO API.
type APIKey struct {
	ID          int     `json:"id,omitempty"`
	Name        string  `json:"name"`
	Active      Bool    `json:"active"`
	Expires     *string `json:"expires"`
	UserID      int     `json:"userID,omitempty"`
	CreatedDate string  `json:"createdDate,omitempty"`
	Status      string  `json:"status,omitempty"`
}

// ServerInfo is the payload of the `info` push event.
type ServerInfo struct {
	Version           string `json:"version"`
	LatestVersion     string `json:"latestVersion"`
	PrimaryBaseURL    string `json:"primaryBaseURL"`
	ServerTimezone    string `json:"serverTimezone"`
	ServerTimezoneOff string `json:"serverTimezoneOffset"`
	IsContainer       Bool   `json:"isContainer"`
	DBType            string `json:"dbType"`
}

// StatusPage is a public status page.
//
// Three columns are deliberately absent: `published`, `search_engine_index` and
// `password`. The saveStatusPage handler has the assignments for those commented
// out in the upstream source, so no event can change them — they are read-only
// as far as any API client is concerned.
type StatusPage struct {
	ID          int     `json:"id,omitempty"`
	Slug        string  `json:"slug"`
	Title       string  `json:"title"`
	Description *string `json:"description"`

	// Icon is either a path the server already stored or a URL. Writes go
	// through the separate imgDataUrl argument of saveStatusPage.
	Icon *string `json:"icon,omitempty"`

	Theme               string `json:"theme"`
	AutoRefreshInterval *int   `json:"autoRefreshInterval,omitempty"`

	ShowTags              *Bool `json:"showTags,omitempty"`
	ShowPoweredBy         *Bool `json:"showPoweredBy,omitempty"`
	ShowCertificateExpiry *Bool `json:"showCertificateExpiry,omitempty"`
	ShowOnlyLastHeartbeat *Bool `json:"showOnlyLastHeartbeat,omitempty"`

	CustomCSS  *string `json:"customCSS"`
	FooterText *string `json:"footerText"`
	RSSTitle   *string `json:"rssTitle"`

	AnalyticsID        *string `json:"analyticsId"`
	AnalyticsScriptURL *string `json:"analyticsScriptUrl"`
	AnalyticsType      *string `json:"analyticsType"`

	// DomainNameList holds the custom domains mapped to this page.
	DomainNameList []string `json:"domainNameList"`

	// Published is reported by the server but cannot be written.
	Published *Bool `json:"published,omitempty"`
}

// StatusPageGroup is one section of a status page. Order matters: the server
// derives each group's weight from its position in the list.
type StatusPageGroup struct {
	ID          int                 `json:"id,omitempty"`
	Name        string              `json:"name"`
	Weight      int                 `json:"weight,omitempty"`
	MonitorList []StatusPageMonitor `json:"monitorList"`
}

// StatusPageMonitor is a monitor shown inside a group. Order matters here too.
type StatusPageMonitor struct {
	ID   int    `json:"id"`
	Name string `json:"name,omitempty"`

	// SendURL controls whether the page links to the monitor's URL.
	SendURL *Bool `json:"sendUrl,omitempty"`
	// URL overrides the link. On write it becomes monitor_group.custom_url; on
	// read the server only reports it when SendURL is true.
	URL *string `json:"url,omitempty"`
}

// StatusPageIncident is the banner pinned to the top of a status page.
type StatusPageIncident struct {
	ID      int    `json:"id,omitempty"`
	Title   string `json:"title"`
	Content string `json:"content"`
	Style   string `json:"style"`

	Pin             *Bool  `json:"pin,omitempty"`
	Active          *Bool  `json:"active,omitempty"`
	CreatedDate     string `json:"createdDate,omitempty"`
	LastUpdatedDate string `json:"lastUpdatedDate,omitempty"`
	StatusPageID    int    `json:"status_page_id,omitempty"`
}
