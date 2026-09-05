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
	PanelAccess        int64  `gorm:"column:panel_access;not null;default:1"`
	LastLoginAt        int64  `gorm:"column:last_login_at;not null;default:0"`
	AvatarMime         string `gorm:"column:avatar_mime;not null;default:''"`
	AvatarPNG          []byte `gorm:"column:avatar_png"`
	// AvatarBlob - хеш картинки в blobs. Одна и та же аватарка у десятка
	// аккаунтов лежит одним блобом, а не десятком копий нахуй
	AvatarBlob    string `gorm:"column:avatar_blob;not null;default:''"`
	AvatarVersion int64  `gorm:"column:avatar_version;not null;default:0"`
	VKLinks       string `gorm:"column:vk_links;not null;default:''"`
	// SuspendedAtUnix cuts this admin out of the invite tree. Zero means active.
	// A suspension is inherited by everyone below: an invite tree is only worth
	// having if a bad branch can be taken off at any point in it.
	SuspendedAtUnix int64  `gorm:"column:suspended_at;not null;default:0"`
	SuspendedReason string `gorm:"column:suspended_reason;not null;default:''"`
	SuspendedByRoot int64  `gorm:"column:suspended_by_root;not null;default:0"`
	// PanelRequestedAt - когда участник попросил открыть ему админ-панель.
	// Ноль означает, что он не просил: заявка снимается вместе с решением по ней
	PanelRequestedAt int64 `gorm:"column:panel_requested_at;not null;default:0"`
	// AccountSubject - кто это по версии нашего провайдера входа. Указатель,
	// потому что уникальность обязана пускать сколько угодно админов вообще без
	// привязки, а это умеет только NULL: пустая строка столкнётся с соседней
	AccountSubject *string `gorm:"column:account_subject;uniqueIndex"`
	// AccountName - как человека зовут у провайдера. Держим для показа, решает
	// всё равно subject: имя у провайдера меняется, номер нет
	AccountName   string `gorm:"column:account_name;not null;default:''"`
	CreatedAtUnix int64  `gorm:"column:created_at;not null"`
	UpdatedAtUnix int64  `gorm:"column:updated_at;not null"`
}

func (Admin) TableName() string { return "admins" }

// Blob - содержимое файла, на которое ссылаются по его же хешу.
//
// Ключ - SHA-512/256: length-extension его не берёт, а на 64-битных он ещё и
// быстрее обычного SHA-256. Одинаковые загрузки схлопываются в одну строку -
// хеш-то у них один
type Blob struct {
	Hash          string `gorm:"column:hash;primaryKey"`
	Mime          string `gorm:"column:mime;not null;default:''"`
	Data          []byte `gorm:"column:data"`
	Size          int64  `gorm:"column:size;not null;default:0"`
	CreatedAtUnix int64  `gorm:"column:created_at;not null"`
}

func (Blob) TableName() string { return "blobs" }

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
	LastPeerIP              string `gorm:"column:last_peer_ip;not null;default:''"`
	LogRuntimeEnabled       int64  `gorm:"column:log_runtime_enabled;not null;default:1"`
	LogProxyEnabled         int64  `gorm:"column:log_proxy_enabled;not null;default:1"`
	LogXrayEnabled          int64  `gorm:"column:log_xray_enabled;not null;default:0"`
	SyncMode                string `gorm:"column:sync_mode;not null;default:'always'"`
	PeriodicIntervalMinutes int64  `gorm:"column:periodic_interval_minutes;not null;default:30"`
	HasRootAccess           int64  `gorm:"column:has_root_access;not null;default:0"`
	VKOAuthAuthorized       int64  `gorm:"column:vk_oauth_authorized;not null;default:0"`
	RemoteControl           int64  `gorm:"column:remote_control;not null;default:1"`
	// Disabled cuts a managed client off: the panel deprovisions its wg peers and
	// ResolveClientConfig refuses to re-provision while it is 1.
	Disabled int64 `gorm:"column:disabled;not null;default:0"`
	// TrafficLimitBytes is the managed client's traffic cap; 0 means unlimited.
	// Usage is accumulated durably in client_traffic; on reaching the cap the
	// panel deprovisions the peers, same as a manual disable.
	TrafficLimitBytes uint64 `gorm:"column:traffic_limit_bytes;not null;default:0"`
}

func (Client) TableName() string { return "clients" }

// ClientTraffic is a managed client's durable traffic accumulator, independent
// of the per-peer counters (which reset when a relay restarts or a peer is
// recreated). The collector adds the non-negative delta of the client's summed
// peer rx/tx each poll; a manual or periodic reset zeroes used_bytes and
// re-baselines last_rx/last_tx. reset_period_days > 0 schedules the next auto
// reset at next_reset_unix.
type ClientTraffic struct {
	ClientID        string `gorm:"column:client_id;primaryKey"`
	UsedBytes       uint64 `gorm:"column:used_bytes;not null;default:0"`
	LastRx          uint64 `gorm:"column:last_rx;not null;default:0"`
	LastTx          uint64 `gorm:"column:last_tx;not null;default:0"`
	ResetPeriodDays int64  `gorm:"column:reset_period_days;not null;default:0"`
	NextResetUnix   int64  `gorm:"column:next_reset_unix;not null;default:0"`
}

func (ClientTraffic) TableName() string { return "client_traffic" }

type ClientConfig struct {
	ClientID      string `gorm:"column:client_id;primaryKey"`
	ConfigProto   []byte `gorm:"column:config_proto;not null"`
	Revision      string `gorm:"column:revision;not null"`
	UpdatedAtUnix int64  `gorm:"column:updated_at;not null"`
	ConfigVersion int64  `gorm:"column:config_version;not null"`
	// TouchedFields - когда какое поле правили в последний раз, путь к версии.
	// По ней считается конфликт: правка соседнего поля никому не мешает, и
	// гонять человека разбираться с ней незачем
	TouchedFields string `gorm:"column:touched_fields;not null;default:''"`
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
	// MaxUses - сколько человек может зарегистрироваться по этому коду. 1 это
	// обычный одноразовый инвайт; больше - код на группу.
	//
	// Лимит НЕ рекурсивный: он ограничивает только тех, кто пришёл по самому
	// коду, а сколько людей приведут они дальше - их дело и их собственные коды.
	MaxUses int64 `gorm:"column:max_uses;not null;default:1"`
	// UseCount растёт при каждом погашении. UsedAt остаётся временем ПЕРВОГО
	// использования, чтобы старые записи читались как раньше.
	UseCount int64 `gorm:"column:use_count;not null;default:0"`
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
	// RelayVersion is the vk-turn-proxy relay's build version (git tag), polled
	// from its management gRPC status for display.
	RelayVersion string `gorm:"column:relay_version;not null;default:''"`
	// WGBackend selects how a vk-turn-proxy node provisions a managed client's
	// WireGuard config: "own" (a peer on the node's own wg interface via the relay
	// management gRPC) or "xui" (a client on a 3x-ui node's inbound). Always set
	// explicitly. XuiNodeID / XuiInboundTag name the 3x-ui node + inbound when
	// WGBackend is "xui".
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

// AdminTrafficRollup is what a member of the invite tree moved, and what
// everybody below them moved. Recomputed by a periodic job rather than joined on
// the fly: it needs every client, their traffic and a walk of the whole tree, and
// doing that on a page load makes the page cost grow with the federation.
//
// Monitoring only. Nothing enforces on subtree_bytes - a personal limit is what
// cuts somebody off, and holding a person responsible for the traffic of people
// they invited a year ago is not a rule anybody could live with.
type AdminTrafficRollup struct {
	AdminID        int64  `gorm:"column:admin_id;primaryKey"`
	OwnBytes       uint64 `gorm:"column:own_bytes;not null;default:0"`
	SubtreeBytes   uint64 `gorm:"column:subtree_bytes;not null;default:0"`
	OwnClients     int64  `gorm:"column:own_clients;not null;default:0"`
	SubtreeClients int64  `gorm:"column:subtree_clients;not null;default:0"`
	SubtreeAdmins  int64  `gorm:"column:subtree_admins;not null;default:0"`
	UpdatedAtUnix  int64  `gorm:"column:updated_at;not null;default:0"`
}

func (AdminTrafficRollup) TableName() string { return "admin_traffic_rollup" }

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
		&Blob{},
		&Donation{},
		&AdminSession{},
		&Client{},
		&InviteToken{},
		&AdminTrafficRollup{},
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
		&ClientTraffic{},
		&InviteRedemption{},
		&AppSession{},
		&AdminTOTP{},
		&AdminTOTPBackup{},
	}
}

// AutoMigrate creates any missing tables, columns, and indexes for every model.
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(All()...)
}

// Donation - занос деньгами в общий котёл или на разработку.
//
// Держим у себя, а не только в башке: башка про аккаунты не знает ничего, а
// человеку надо показать его же историю заносов
type Donation struct {
	ID      uint64 `gorm:"column:id;primaryKey"`
	AdminID int64  `gorm:"column:admin_id;index;not null"`
	// Kind - traffic или dev. Первый идёт в общий котёл и греет доверие, второй
	// нам на разработку и не греет нихуя
	Kind string `gorm:"column:kind;not null;default:'traffic'"`
	// Reference - подпись транзакции, уникальна: один занос засчитывается ровно
	// раз, иначе доверие раздаётся за чужие деньги
	Reference   string `gorm:"column:reference;uniqueIndex;not null"`
	AmountMicro int64  `gorm:"column:amount_micro;not null"`
	AtUnix      int64  `gorm:"column:at;not null"`
}

// TableName задаёт имя таблицы
func (Donation) TableName() string { return "donations" }

// InviteRedemption - один погашенный код одним аккаунтом
type InviteRedemption struct {
	Token         string `gorm:"column:token;primaryKey"`
	AdminID       int64  `gorm:"column:admin_id;primaryKey"`
	CreatedAtUnix int64  `gorm:"column:created_at;not null"`
}

// TableName задаёт имя таблицы
func (InviteRedemption) TableName() string { return "invite_redemptions" }

// AppSession - сессия приложения на одном устройстве
type AppSession struct {
	ID            string `gorm:"column:id;primaryKey"`
	AdminID       int64  `gorm:"column:admin_id;index;not null"`
	TokenHash     string `gorm:"column:token_hash;uniqueIndex;not null"`
	DeviceName    string `gorm:"column:device_name;not null;default:''"`
	CreatedAtUnix int64  `gorm:"column:created_at;not null"`
	LastSeenAt    int64  `gorm:"column:last_seen_at;not null;default:0"`
	ExpiresAt     int64  `gorm:"column:expires_at;not null"`
}

// TableName задаёт имя таблицы
func (AppSession) TableName() string { return "app_sessions" }

// AdminTOTP - 2FA аккаунта
type AdminTOTP struct {
	AdminID       int64  `gorm:"column:admin_id;primaryKey"`
	Secret        string `gorm:"column:secret;not null"`
	ConfirmedAt   int64  `gorm:"column:confirmed_at;not null;default:0"`
	CreatedAtUnix int64  `gorm:"column:created_at;not null"`
}

// TableName задаёт имя таблицы
func (AdminTOTP) TableName() string { return "admin_totp" }

// AdminTOTPBackup - резервный код
type AdminTOTPBackup struct {
	AdminID  int64  `gorm:"column:admin_id;primaryKey"`
	CodeHash string `gorm:"column:code_hash;primaryKey"`
	UsedAt   int64  `gorm:"column:used_at;not null;default:0"`
}

// TableName задаёт имя таблицы
func (AdminTOTPBackup) TableName() string { return "admin_totp_backup" }
