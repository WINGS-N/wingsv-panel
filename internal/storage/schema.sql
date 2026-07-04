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
    vk_links TEXT NOT NULL DEFAULT '',
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
    periodic_interval_minutes INTEGER NOT NULL DEFAULT 30,
    remote_control INTEGER NOT NULL DEFAULT 1
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

-- Pending guardian commands queued while the device was offline. Replayed on
-- the next welcome handshake. Only stores commands that make sense
-- regardless of when they fire (refresh_*, generate_vk_link) - state-of-the-
-- moment commands like start/stop/reconnect bypass the queue entirely.
CREATE TABLE IF NOT EXISTS pending_commands (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    client_id TEXT NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    command_type INTEGER NOT NULL,
    subscription_id TEXT NOT NULL DEFAULT '',
    queued_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_pending_commands_client ON pending_commands(client_id, queued_at);

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

-- Backend servers the panel manages in wg/awg and 3x-ui modes. kind is
-- 'vk_turn_proxy' or 'xui'; ca_pin is the SPKI pin the panel uses to verify the
-- node's gRPC TLS.
CREATE TABLE IF NOT EXISTS server_nodes (
    id TEXT PRIMARY KEY,
    kind TEXT NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    grpc_endpoint TEXT NOT NULL DEFAULT '',
    grpc_token TEXT NOT NULL DEFAULT '',
    ca_pin BLOB,
    status TEXT NOT NULL DEFAULT 'unknown',
    owner_admin_id INTEGER NOT NULL DEFAULT 0,
    last_seen_at INTEGER NOT NULL DEFAULT 0,
    xray_state TEXT NOT NULL DEFAULT '',
    xray_version TEXT NOT NULL DEFAULT '',
    wg_backend TEXT NOT NULL DEFAULT '',
    xui_node_id TEXT NOT NULL DEFAULT '',
    xui_inbound_tag TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_server_nodes_kind ON server_nodes(kind);

-- WireGuard/AmneziaWG peer a client holds on a given vk-turn-proxy node. One
-- client may hold peers on several nodes (HA). private_key is stored only when
-- the node generated the keypair so the panel can hand it to the client.
CREATE TABLE IF NOT EXISTS client_wg_peers (
    client_id TEXT NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    node_id TEXT NOT NULL REFERENCES server_nodes(id) ON DELETE CASCADE,
    public_key TEXT NOT NULL DEFAULT '',
    private_key TEXT NOT NULL DEFAULT '',
    allowed_ips TEXT NOT NULL DEFAULT '',
    server_public_key TEXT NOT NULL DEFAULT '',
    endpoint TEXT NOT NULL DEFAULT '',
    created_at INTEGER NOT NULL,
    PRIMARY KEY (client_id, node_id)
);

-- M4 stats: one time-series point per node per poll (traffic totals + counts).
CREATE TABLE IF NOT EXISTS traffic_samples (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    node_id TEXT NOT NULL,
    ts INTEGER NOT NULL,
    rx_bytes INTEGER NOT NULL DEFAULT 0,
    tx_bytes INTEGER NOT NULL DEFAULT 0,
    active_streams INTEGER NOT NULL DEFAULT 0,
    active_sessions INTEGER NOT NULL DEFAULT 0,
    peer_count INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_traffic_node_ts ON traffic_samples(node_id, ts);

-- M4 stats: current live relay flows, replaced wholesale each poll.
CREATE TABLE IF NOT EXISTS flow_snapshots (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    node_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    stream_id INTEGER NOT NULL,
    client_ip TEXT NOT NULL DEFAULT '',
    remote TEXT NOT NULL DEFAULT '',
    protocol TEXT NOT NULL DEFAULT '',
    version INTEGER NOT NULL DEFAULT 0,
    rx_bytes INTEGER NOT NULL DEFAULT 0,
    tx_bytes INTEGER NOT NULL DEFAULT 0,
    rx_rate INTEGER NOT NULL DEFAULT 0,
    tx_rate INTEGER NOT NULL DEFAULT 0,
    started_unix INTEGER NOT NULL DEFAULT 0,
    sampled_unix INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_flow_snapshots_node ON flow_snapshots(node_id);

-- M4 stats: append/refresh history of relay flows for the connection log.
CREATE TABLE IF NOT EXISTS connection_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    node_id TEXT NOT NULL,
    session_id TEXT NOT NULL,
    stream_id INTEGER NOT NULL,
    started_unix INTEGER NOT NULL,
    client_ip TEXT NOT NULL DEFAULT '',
    remote TEXT NOT NULL DEFAULT '',
    protocol TEXT NOT NULL DEFAULT '',
    rx_bytes INTEGER NOT NULL DEFAULT 0,
    tx_bytes INTEGER NOT NULL DEFAULT 0,
    first_seen INTEGER NOT NULL,
    last_seen INTEGER NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_conn_identity ON connection_log(node_id, session_id, stream_id, started_unix);
CREATE INDEX IF NOT EXISTS idx_conn_first_seen ON connection_log(first_seen);
CREATE INDEX IF NOT EXISTS idx_conn_last_seen ON connection_log(last_seen);

-- M4 stats: per-peer counters, joined with client_wg_peers for per-client traffic.
CREATE TABLE IF NOT EXISTS peer_traffic (
    node_id TEXT NOT NULL,
    public_key TEXT NOT NULL,
    rx_bytes INTEGER NOT NULL DEFAULT 0,
    tx_bytes INTEGER NOT NULL DEFAULT 0,
    sampled_unix INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (node_id, public_key)
);
