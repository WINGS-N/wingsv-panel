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
	ID            string `gorm:"column:id;primaryKey"`
	Kind          string `gorm:"column:kind;not null;index"`
	Name          string `gorm:"column:name;not null"`
	GRPCEndpoint  string `gorm:"column:grpc_endpoint;not null"`
	CAPin         []byte `gorm:"column:ca_pin"`
	Status        string `gorm:"column:status;not null"`
	LastSeenAt    int64  `gorm:"column:last_seen_at;not null;default:0"`
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
	}
}

// AutoMigrate creates any missing tables, columns, and indexes for every model.
func AutoMigrate(db *gorm.DB) error {
	return db.AutoMigrate(All()...)
}
