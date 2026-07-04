// Package dbmodel holds the GORM row models for every panel table. They map
// one-to-one onto the columns in storage/schema.sql and are the single schema
// source for pgsql / mariadb (via AutoMigrate) and the row-copy engine behind
// the "db migrate" command. The runtime query methods are being ported onto
// these types incrementally.
package dbmodel

import "gorm.io/gorm"

type Admin struct {
	ID                 int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Username           string `gorm:"column:username;uniqueIndex;not null"`
	PasswordHash       string `gorm:"column:password_hash;not null"`
	MustChangePassword int64  `gorm:"column:must_change_password;not null;default:0"`
	Role               string `gorm:"column:role;not null;default:'admin'"`
	LastLoginAt        int64  `gorm:"column:last_login_at;not null;default:0"`
	AvatarMime         string `gorm:"column:avatar_mime;not null;default:''"`
	AvatarPNG          []byte `gorm:"column:avatar_png"`
	AvatarVersion      int64  `gorm:"column:avatar_version;not null;default:0"`
	VKLinks            string `gorm:"column:vk_links;not null;default:''"`
	CreatedAtUnix      int64  `gorm:"column:created_at;not null"`
	UpdatedAtUnix      int64  `gorm:"column:updated_at;not null"`
}

func (Admin) TableName() string { return "admins" }

type AdminSession struct {
	ID            string `gorm:"column:id;primaryKey"`
	AdminID       int64  `gorm:"column:admin_id;not null;index"`
	ExpiresAt     int64  `gorm:"column:expires_at;not null;index"`
	CreatedAtUnix int64  `gorm:"column:created_at;not null"`
}

func (AdminSession) TableName() string { return "admin_sessions" }

type Client struct {
	ID                      string `gorm:"column:id;primaryKey"`
	OwnerAdminID            int64  `gorm:"column:owner_admin_id;not null;index"`
	Name                    string `gorm:"column:name;not null"`
	TokenHash               string `gorm:"column:token_hash;not null"`
	TokenPlain              []byte `gorm:"column:token_plain"`
	HWID                    string `gorm:"column:hwid;not null;default:''"`
	DeviceName              string `gorm:"column:device_name;not null;default:''"`
	DeviceModel             string `gorm:"column:device_model;not null;default:''"`
	OSVersion               string `gorm:"column:os_version;not null;default:''"`
	AppVersion              string `gorm:"column:app_version;not null;default:''"`
	CreatedAtUnix           int64  `gorm:"column:created_at;not null"`
	LastSeenAt              int64  `gorm:"column:last_seen_at;not null;default:0"`
	Online                  int64  `gorm:"column:online;not null;default:0"`
	LogRuntimeEnabled       int64  `gorm:"column:log_runtime_enabled;not null;default:1"`
	LogProxyEnabled         int64  `gorm:"column:log_proxy_enabled;not null;default:1"`
	LogXrayEnabled          int64  `gorm:"column:log_xray_enabled;not null;default:0"`
	SyncMode                string `gorm:"column:sync_mode;not null;default:'always'"`
	PeriodicIntervalMinutes int64  `gorm:"column:periodic_interval_minutes;not null;default:30"`
	HasRootAccess           int64  `gorm:"column:has_root_access;not null;default:0"`
	VKOAuthAuthorized       int64  `gorm:"column:vk_oauth_authorized;not null;default:0"`
	RemoteControl           int64  `gorm:"column:remote_control;not null;default:1"`
}

func (Client) TableName() string { return "clients" }

type ClientConfig struct {
	ClientID      string `gorm:"column:client_id;primaryKey"`
	ConfigProto   []byte `gorm:"column:config_proto;not null"`
	Revision      string `gorm:"column:revision;not null"`
	UpdatedAtUnix int64  `gorm:"column:updated_at;not null"`
	ConfigVersion int64  `gorm:"column:config_version;not null"`
}

func (ClientConfig) TableName() string { return "client_configs" }

type ClientReportedConfig struct {
	ClientID      string `gorm:"column:client_id;primaryKey"`
	ConfigProto   []byte `gorm:"column:config_proto;not null"`
	UpdatedAtUnix int64  `gorm:"column:updated_at;not null"`
}

func (ClientReportedConfig) TableName() string { return "client_reported_configs" }

type ClientRuntime struct {
	ClientID      string `gorm:"column:client_id;primaryKey"`
	RuntimeProto  []byte `gorm:"column:runtime_proto;not null"`
	UpdatedAtUnix int64  `gorm:"column:updated_at;not null"`
}

func (ClientRuntime) TableName() string { return "client_runtime" }

type ClientLog struct {
	ClientID string `gorm:"column:client_id;primaryKey"`
	Stream   int64  `gorm:"column:stream;primaryKey"`
	Seq      int64  `gorm:"column:seq;primaryKey"`
	Ts       int64  `gorm:"column:ts;not null"`
	Text     string `gorm:"column:text;not null"`
}

func (ClientLog) TableName() string { return "client_logs" }

type KV struct {
	Key   string `gorm:"column:key;primaryKey"`
	Value []byte `gorm:"column:value;not null"`
}

func (KV) TableName() string { return "kv" }

type ClientInstalledApps struct {
	ClientID      string `gorm:"column:client_id;primaryKey"`
	AppsProto     []byte `gorm:"column:apps_proto;not null"`
	UpdatedAtUnix int64  `gorm:"column:updated_at;not null"`
}

func (ClientInstalledApps) TableName() string { return "client_installed_apps" }

type PendingCommand struct {
	ID             int64  `gorm:"column:id;primaryKey;autoIncrement"`
	ClientID       string `gorm:"column:client_id;not null;index"`
	CommandType    int64  `gorm:"column:command_type;not null"`
	SubscriptionID string `gorm:"column:subscription_id;not null;default:''"`
	QueuedAt       int64  `gorm:"column:queued_at;not null"`
	ExpiresAt      int64  `gorm:"column:expires_at;not null"`
}

func (PendingCommand) TableName() string { return "pending_commands" }

type PackageMetadata struct {
	Package       string `gorm:"column:package;primaryKey"`
	Label         string `gorm:"column:label;not null"`
	IconPNG       []byte `gorm:"column:icon_png"`
	UpdatedAtUnix int64  `gorm:"column:updated_at;not null"`
}

func (PackageMetadata) TableName() string { return "package_metadata" }

type AuditLog struct {
	ID            int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Ts            int64  `gorm:"column:ts;not null;index"`
	ActorAdminID  *int64 `gorm:"column:actor_admin_id"`
	ActorUsername string `gorm:"column:actor_username;not null"`
	Action        string `gorm:"column:action;not null"`
	TargetType    string `gorm:"column:target_type;not null"`
	TargetID      string `gorm:"column:target_id;not null"`
	Message       string `gorm:"column:message;not null"`
	IP            string `gorm:"column:ip;not null"`
}

func (AuditLog) TableName() string { return "audit_log" }

type PlatformSetting struct {
	Key   string `gorm:"column:key;primaryKey"`
	Value string `gorm:"column:value;not null"`
}

func (PlatformSetting) TableName() string { return "platform_settings" }

type InviteToken struct {
	Token            string `gorm:"column:token;primaryKey"`
	CreatedAtUnix    int64  `gorm:"column:created_at;not null"`
	ExpiresAt        int64  `gorm:"column:expires_at;not null"`
	UsedAt           int64  `gorm:"column:used_at;not null;default:0"`
	UsedByAdminID    *int64 `gorm:"column:used_by_admin_id"`
	CreatedByAdminID *int64 `gorm:"column:created_by_admin_id"`
}

func (InviteToken) TableName() string { return "invite_tokens" }

type AdminMasterConfig struct {
	AdminID                 int64  `gorm:"column:admin_id;primaryKey"`
	ConfigProto             []byte `gorm:"column:config_proto"`
	SyncMode                string `gorm:"column:sync_mode;not null;default:'always'"`
	PeriodicIntervalMinutes int64  `gorm:"column:periodic_interval_minutes;not null;default:30"`
	ScopeFlags              string `gorm:"column:scope_flags;not null"`
	UpdatedAtUnix           int64  `gorm:"column:updated_at;not null"`
}

func (AdminMasterConfig) TableName() string { return "admin_master_config" }

type ServerNode struct {
	ID           string `gorm:"column:id;primaryKey"`
	Kind         string `gorm:"column:kind;not null;index"`
	Name         string `gorm:"column:name;not null"`
	GRPCEndpoint string `gorm:"column:grpc_endpoint;not null"`
	// GRPCToken is the bearer credential the node's Relay/Panel gRPC checks. Empty
	// for a panel-local node (the collector falls back to the configured relay
	// token); set for an admin's own external endpoint.
	GRPCToken string `gorm:"column:grpc_token;not null;default:''"`
	CAPin     []byte `gorm:"column:ca_pin"`
	Status    string `gorm:"column:status;not null"`
	// OwnerAdminID is 0 for a panel-local node the owner deploys and manages
	// (its provisioning and stats are owner-only), or the id of the admin who
	// registered it as their own external vk-turn-proxy / 3x-ui gRPC endpoint.
	OwnerAdminID int64 `gorm:"column:owner_admin_id;not null;default:0;index"`
	LastSeenAt   int64 `gorm:"column:last_seen_at;not null;default:0"`
	// XrayState / XrayVersion are polled from a 3x-ui node's Panel gRPC so the UI
	// can show whether its Xray core is running. Empty for vk-turn-proxy nodes.
	XrayState   string `gorm:"column:xray_state;not null;default:''"`
	XrayVersion string `gorm:"column:xray_version;not null;default:''"`
	// WGBackend selects how a vk-turn-proxy node provisions a managed client's
	// WireGuard config: "own" (a peer on the node's own wg interface via the relay
	// management gRPC) or "xui" (a client on a 3x-ui node's inbound). Empty falls
	// back to the panel's global XUI_WG_* default. XuiNodeID / XuiInboundTag name
	// the 3x-ui node + inbound when WGBackend is "xui".
	WGBackend     string `gorm:"column:wg_backend;not null;default:''"`
	XuiNodeID     string `gorm:"column:xui_node_id;not null;default:''"`
	XuiInboundTag string `gorm:"column:xui_inbound_tag;not null;default:''"`
	CreatedAtUnix int64  `gorm:"column:created_at;not null"`
}

func (ServerNode) TableName() string { return "server_nodes" }

type ClientWGPeer struct {
	ClientID        string `gorm:"column:client_id;primaryKey"`
	NodeID          string `gorm:"column:node_id;primaryKey"`
	PublicKey       string `gorm:"column:public_key;not null"`
	PrivateKey      string `gorm:"column:private_key;not null"`
	AllowedIPs      string `gorm:"column:allowed_ips;not null"`
	ServerPublicKey string `gorm:"column:server_public_key;not null"`
	Endpoint        string `gorm:"column:endpoint;not null"`
	CreatedAtUnix   int64  `gorm:"column:created_at;not null"`
}

func (ClientWGPeer) TableName() string { return "client_wg_peers" }

// TrafficSample is one time-series point per server node, written by the stats
// collector each poll. For a vk-turn-proxy node the byte counters are the relay's
// cumulative server_rx/tx_bytes and the session/stream/peer counts come from its
// flow stats; for a 3x-ui node the counters carry the aggregate inbound up/down
// and ActiveSessions the online-client count.
type TrafficSample struct {
	ID             uint64 `gorm:"column:id;primaryKey;autoIncrement"`
	NodeID         string `gorm:"column:node_id;not null;index:idx_traffic_node_ts,priority:1"`
	TsUnix         int64  `gorm:"column:ts;not null;index:idx_traffic_node_ts,priority:2"`
	RxBytes        uint64 `gorm:"column:rx_bytes;not null;default:0"`
	TxBytes        uint64 `gorm:"column:tx_bytes;not null;default:0"`
	ActiveStreams  uint32 `gorm:"column:active_streams;not null;default:0"`
	ActiveSessions uint32 `gorm:"column:active_sessions;not null;default:0"`
	PeerCount      uint32 `gorm:"column:peer_count;not null;default:0"`
}

func (TrafficSample) TableName() string { return "traffic_samples" }

// FlowSnapshot is one live relay flow captured at the last poll. The collector
// replaces a node's whole snapshot each cycle, so this table is the current state
// the flow-chain view renders (client_ip -> session/stream -> remote).
type FlowSnapshot struct {
	ID          uint64 `gorm:"column:id;primaryKey;autoIncrement"`
	NodeID      string `gorm:"column:node_id;not null;index"`
	SessionID   string `gorm:"column:session_id;not null"`
	StreamID    uint32 `gorm:"column:stream_id;not null"`
	ClientIP    string `gorm:"column:client_ip;not null;default:''"`
	Remote      string `gorm:"column:remote;not null;default:''"`
	Protocol    string `gorm:"column:protocol;not null;default:''"`
	Version     uint32 `gorm:"column:version;not null;default:0"`
	RxBytes     uint64 `gorm:"column:rx_bytes;not null;default:0"`
	TxBytes     uint64 `gorm:"column:tx_bytes;not null;default:0"`
	RxRate      uint64 `gorm:"column:rx_rate;not null;default:0"`
	TxRate      uint64 `gorm:"column:tx_rate;not null;default:0"`
	StartedUnix int64  `gorm:"column:started_unix;not null;default:0"`
	SampledUnix int64  `gorm:"column:sampled_unix;not null;default:0"`
}

func (FlowSnapshot) TableName() string { return "flow_snapshots" }

// ConnectionLog is the append/refresh history of relay flows: one row per
// (node, session, stream, start), first_seen set on insert and last_seen plus the
// byte counters refreshed while the flow stays live. The connection-log view reads
// it newest-first; a retention prune drops old rows.
type ConnectionLog struct {
	ID            uint64 `gorm:"column:id;primaryKey;autoIncrement"`
	NodeID        string `gorm:"column:node_id;not null;uniqueIndex:idx_conn_identity,priority:1"`
	SessionID     string `gorm:"column:session_id;not null;uniqueIndex:idx_conn_identity,priority:2"`
	StreamID      uint32 `gorm:"column:stream_id;not null;uniqueIndex:idx_conn_identity,priority:3"`
	StartedUnix   int64  `gorm:"column:started_unix;not null;uniqueIndex:idx_conn_identity,priority:4"`
	ClientIP      string `gorm:"column:client_ip;not null;default:''"`
	Remote        string `gorm:"column:remote;not null;default:''"`
	Protocol      string `gorm:"column:protocol;not null;default:''"`
	RxBytes       uint64 `gorm:"column:rx_bytes;not null;default:0"`
	TxBytes       uint64 `gorm:"column:tx_bytes;not null;default:0"`
	FirstSeenUnix int64  `gorm:"column:first_seen;not null;index"`
	LastSeenUnix  int64  `gorm:"column:last_seen;not null;index"`
}

func (ConnectionLog) TableName() string { return "connection_log" }

// PeerTraffic is a wg peer's latest cumulative byte counters on a node, refreshed
// each poll. Joined with client_wg_peers (node_id + public_key) it attributes
// traffic to a managed client for the per-client traffic column.
type PeerTraffic struct {
	NodeID      string `gorm:"column:node_id;primaryKey"`
	PublicKey   string `gorm:"column:public_key;primaryKey"`
	RxBytes     uint64 `gorm:"column:rx_bytes;not null;default:0"`
	TxBytes     uint64 `gorm:"column:tx_bytes;not null;default:0"`
	SampledUnix int64  `gorm:"column:sampled_unix;not null;default:0"`
}

func (PeerTraffic) TableName() string { return "peer_traffic" }

// NodeTrafficTotal is a per-node persistent accumulator of transferred bytes, so
// all-time totals survive sample-retention pruning and relay counter resets.
// last_rx/last_tx hold the previous cumulative reading; each poll adds the
// non-negative delta (a smaller reading means the relay restarted, adding zero).
type NodeTrafficTotal struct {
	NodeID  string `gorm:"column:node_id;primaryKey"`
	RxTotal uint64 `gorm:"column:rx_total;not null;default:0"`
	TxTotal uint64 `gorm:"column:tx_total;not null;default:0"`
	LastRx  uint64 `gorm:"column:last_rx;not null;default:0"`
	LastTx  uint64 `gorm:"column:last_tx;not null;default:0"`
}

func (NodeTrafficTotal) TableName() string { return "node_traffic_total" }

// All returns every model in parent-first order so a row copy inserts referenced
// rows before the rows that point at them.
func All() []any {
	return []any{
		&Admin{},
		&AdminSession{},
		&Client{},
		&InviteToken{},
		&AuditLog{},
		&AdminMasterConfig{},
		&ClientConfig{},
		&ClientReportedConfig{},
		&ClientRuntime{},
		&ClientLog{},
		&ClientInstalledApps{},
		&PendingCommand{},
		&PackageMetadata{},
		&KV{},
		&PlatformSetting{},
		&ServerNode{},
		&ClientWGPeer{},
		&TrafficSample{},
		&FlowSnapshot{},
		&ConnectionLog{},
		&PeerTraffic{},
		&NodeTrafficTotal{},
	}
}

// AutoMigrate creates any missing tables, columns, and indexes for every model.
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(All()...)
}
