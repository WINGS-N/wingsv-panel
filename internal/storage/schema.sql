CREATE TABLE IF NOT EXISTS admins (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    must_change_password INTEGER NOT NULL DEFAULT 0,
    role TEXT NOT NULL DEFAULT 'admin',
    last_login_at INTEGER NOT NULL DEFAULT 0,
    avatar_mime TEXT NOT NULL DEFAULT '',
    avatar_png BLOB,
    avatar_version INTEGER NOT NULL DEFAULT 0,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS admin_sessions (
    id TEXT PRIMARY KEY,
    admin_id INTEGER NOT NULL REFERENCES admins(id) ON DELETE CASCADE,
    expires_at INTEGER NOT NULL,
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_admin_sessions_admin_id ON admin_sessions(admin_id);
CREATE INDEX IF NOT EXISTS idx_admin_sessions_expires_at ON admin_sessions(expires_at);

CREATE TABLE IF NOT EXISTS clients (
    id TEXT PRIMARY KEY,
    owner_admin_id INTEGER NOT NULL REFERENCES admins(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    token_hash TEXT NOT NULL,
    token_plain BLOB,
    hwid TEXT NOT NULL DEFAULT '',
    device_name TEXT NOT NULL DEFAULT '',
    device_model TEXT NOT NULL DEFAULT '',
    os_version TEXT NOT NULL DEFAULT '',
    app_version TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    last_seen_at INTEGER NOT NULL DEFAULT 0,
    online INTEGER NOT NULL DEFAULT 0,
    log_runtime_enabled INTEGER NOT NULL DEFAULT 1,
    log_proxy_enabled INTEGER NOT NULL DEFAULT 1,
    log_xray_enabled INTEGER NOT NULL DEFAULT 0,
    sync_mode TEXT NOT NULL DEFAULT 'always',
    periodic_interval_minutes INTEGER NOT NULL DEFAULT 30
);

CREATE INDEX IF NOT EXISTS idx_clients_owner ON clients(owner_admin_id);

CREATE TABLE IF NOT EXISTS client_configs (
    client_id TEXT PRIMARY KEY REFERENCES clients(id) ON DELETE CASCADE,
    config_proto BLOB NOT NULL,
    revision TEXT NOT NULL,
    updated_at INTEGER NOT NULL,
    config_version INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS client_reported_configs (
    client_id TEXT PRIMARY KEY REFERENCES clients(id) ON DELETE CASCADE,
    config_proto BLOB NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS client_runtime (
    client_id TEXT PRIMARY KEY REFERENCES clients(id) ON DELETE CASCADE,
    runtime_proto BLOB NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS client_logs (
    client_id TEXT NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    stream INTEGER NOT NULL,
    seq INTEGER NOT NULL,
    ts INTEGER NOT NULL,
    text TEXT NOT NULL,
    PRIMARY KEY (client_id, stream, seq)
);

CREATE INDEX IF NOT EXISTS idx_client_logs_lookup ON client_logs(client_id, stream, seq DESC);

CREATE TABLE IF NOT EXISTS kv (
    key TEXT PRIMARY KEY,
    value BLOB NOT NULL
);

CREATE TABLE IF NOT EXISTS client_installed_apps (
    client_id TEXT PRIMARY KEY REFERENCES clients(id) ON DELETE CASCADE,
    apps_proto BLOB NOT NULL,
    updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS package_metadata (
    package TEXT PRIMARY KEY,
    label TEXT NOT NULL DEFAULT '',
    icon_png BLOB,
    updated_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    ts INTEGER NOT NULL,
    actor_admin_id INTEGER REFERENCES admins(id) ON DELETE SET NULL,
    actor_username TEXT NOT NULL DEFAULT '',
    action TEXT NOT NULL,
    target_type TEXT NOT NULL DEFAULT '',
    target_id TEXT NOT NULL DEFAULT '',
    message TEXT NOT NULL DEFAULT '',
    ip TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_audit_log_ts ON audit_log(ts DESC);
CREATE INDEX IF NOT EXISTS idx_audit_log_actor ON audit_log(actor_admin_id, ts DESC);

CREATE TABLE IF NOT EXISTS platform_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS invite_tokens (
    token TEXT PRIMARY KEY,
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL DEFAULT 0,
    used_at INTEGER NOT NULL DEFAULT 0,
    used_by_admin_id INTEGER REFERENCES admins(id) ON DELETE SET NULL,
    created_by_admin_id INTEGER REFERENCES admins(id) ON DELETE SET NULL
);

CREATE INDEX IF NOT EXISTS idx_invite_tokens_created_at ON invite_tokens(created_at DESC);

CREATE TABLE IF NOT EXISTS admin_master_config (
    admin_id INTEGER PRIMARY KEY REFERENCES admins(id) ON DELETE CASCADE,
    config_proto BLOB,
    sync_mode TEXT NOT NULL DEFAULT '',
    periodic_interval_minutes INTEGER NOT NULL DEFAULT 0,
    scope_flags TEXT NOT NULL DEFAULT '',
    updated_at INTEGER NOT NULL DEFAULT 0
);
