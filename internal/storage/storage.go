package storage

import (
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"v.wingsnet.org/internal/storage/dbmodel"
)

//go:embed schema.sql
var schemaSQL string

// Driver is a supported SQL backend. The panel keeps a single portable data
// model across all three via GORM; only the connection layer and DDL differ.
type Driver string

const (
	DriverSQLite   Driver = "sqlite"
	DriverPostgres Driver = "pgsql"
	DriverMySQL    Driver = "mariadb"
)

// Options selects the database backend. For sqlite, DSN is a filesystem path;
// for pgsql / mariadb it is a full driver DSN.
type Options struct {
	Driver Driver
	DSN    string
}

// NormalizeDriver maps the common aliases to the canonical Driver value.
func NormalizeDriver(s string) (Driver, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "sqlite", "sqlite3":
		return DriverSQLite, nil
	case "pgsql", "postgres", "postgresql":
		return DriverPostgres, nil
	case "mariadb", "mysql":
		return DriverMySQL, nil
	default:
		return "", fmt.Errorf("storage: unknown driver %q", s)
	}
}

type Store struct {
	gdb    *gorm.DB
	db     *sql.DB
	driver Driver
}

// Driver reports the active database backend, so callers can tune behaviour that
// matters for sqlite's single-writer file (e.g. how often to persist samples).
func (s *Store) Driver() Driver { return s.driver }

// IsSQLite reports whether the store is backed by the single-writer sqlite file.
func (s *Store) IsSQLite() bool { return s.driver == DriverSQLite }

// Open connects to the configured backend, applies the schema, and runs the
// idempotent data migrations. The pure-Go drivers (glebarez/sqlite, pgx,
// go-sql-driver/mysql) keep the binary CGO-free so it cross-compiles to every
// release arch, including android/arm64+arm and riscv64.
func Open(opts Options) (*Store, error) {
	driver, err := NormalizeDriver(string(opts.Driver))
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(opts.DSN) == "" {
		return nil, errors.New("storage: empty DSN")
	}

	var dialector gorm.Dialector
	switch driver {
	case DriverSQLite:
		// WAL lets many readers run alongside a single writer; busy_timeout waits
		// out write contention instead of erroring; foreign_keys(ON) enforces the
		// ON DELETE CASCADE relations the schema declares.
		dsn := fmt.Sprintf(
			"file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=synchronous(NORMAL)",
			opts.DSN,
		)
		dialector = sqlite.Open(dsn)
	case DriverPostgres:
		dialector = postgres.Open(opts.DSN)
	case DriverMySQL:
		dialector = mysql.Open(opts.DSN)
	}

	gdb, err := gorm.Open(dialector, &gorm.Config{
		Logger:                 gormlogger.Default.LogMode(gormlogger.Silent),
		SkipDefaultTransaction: true,
	})
	if err != nil {
		return nil, fmt.Errorf("storage: open %s: %w", driver, err)
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, fmt.Errorf("storage: sql handle: %w", err)
	}
	// See the WAL note above: real read concurrency, writes serialize at the
	// engine and wait via busy_timeout rather than starving admin requests.
	sqlDB.SetMaxOpenConns(24)
	sqlDB.SetMaxIdleConns(24)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)

	if err := applySchema(sqlDB, driver); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	// sqlite keeps its embedded schema.sql (existing DBs upgrade in place); pgsql
	// and mariadb have no hand-written DDL, so dbmodel is their schema source.
	if driver != DriverSQLite {
		if err := dbmodel.AutoMigrate(gdb); err != nil {
			_ = sqlDB.Close()
			return nil, fmt.Errorf("storage: automigrate %s: %w", driver, err)
		}
	}
	if err := migrateAdminUsernamesToLower(gdb); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("storage: migrate admin usernames: %w", err)
	}
	if err := migrateNodeWGBackends(gdb); err != nil {
		_ = sqlDB.Close()
		return nil, fmt.Errorf("storage: migrate node wg backends: %w", err)
	}
	store := &Store{gdb: gdb, db: sqlDB, driver: driver}
	if err := store.PurgeOrphanClientRows(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return store, nil
}

// applySchema creates tables and adds any missing columns. sqlite is fully
// schema-owned by schema.sql (unchanged from the raw-sql era so existing DBs
// upgrade in place); pgsql / mariadb DDL lands as those backends are wired.
func applySchema(db *sql.DB, driver Driver) error {
	if driver != DriverSQLite {
		return nil
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		return fmt.Errorf("storage: apply schema: %w", err)
	}
	// Idempotent column-adds for upgrades from older schemas. SQLite returns
	// "duplicate column name" when the column already exists; that's expected.
	for _, alter := range []string{
		`ALTER TABLE clients ADD COLUMN token_plain BLOB`,
		`ALTER TABLE clients ADD COLUMN sync_mode TEXT NOT NULL DEFAULT 'always'`,
		`ALTER TABLE clients ADD COLUMN periodic_interval_minutes INTEGER NOT NULL DEFAULT 30`,
		`ALTER TABLE admins ADD COLUMN role TEXT NOT NULL DEFAULT 'admin'`,
		`ALTER TABLE invite_tokens ADD COLUMN max_uses INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE invite_tokens ADD COLUMN use_count INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE admins ADD COLUMN last_login_at INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE admins ADD COLUMN avatar_mime TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE admins ADD COLUMN avatar_png BLOB`,
		`ALTER TABLE admins ADD COLUMN avatar_version INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE client_configs ADD COLUMN config_version INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE clients ADD COLUMN has_root_access INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE clients ADD COLUMN vk_oauth_authorized INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE clients ADD COLUMN remote_control INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE clients ADD COLUMN disabled INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE clients ADD COLUMN traffic_limit_bytes INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE admins ADD COLUMN vk_links TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE admins ADD COLUMN suspended_at INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE admins ADD COLUMN suspended_reason TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE admins ADD COLUMN suspended_by_root INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE admins ADD COLUMN matrix_id TEXT`,
		`ALTER TABLE admins ADD COLUMN matrix_subject TEXT NOT NULL DEFAULT ''`,
		// Доступ в админ-панель отделён от аккаунта: личный доступ к VPN есть у
		// каждого, а панель открывается отдельно. Существующие аккаунты
		// заводились админами, поэтому единица по умолчанию
		`ALTER TABLE admins ADD COLUMN panel_access INTEGER NOT NULL DEFAULT 1`,
		// Заявка участника на панель: ноль означает, что он не просил
		`ALTER TABLE admins ADD COLUMN panel_requested_at INTEGER NOT NULL DEFAULT 0`,
		// Картинки переехали в blobs: в аккаунте остаётся только хеш
		`ALTER TABLE admins ADD COLUMN avatar_blob TEXT NOT NULL DEFAULT ''`,
		// Отметки о правках по полям: по ним считается конфликт двух админов
		`ALTER TABLE client_configs ADD COLUMN touched_fields TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE clients ADD COLUMN last_peer_ip TEXT NOT NULL DEFAULT ''`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_admins_matrix_id ON admins(matrix_id) WHERE matrix_id IS NOT NULL`,
		`ALTER TABLE server_nodes ADD COLUMN owner_admin_id INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE server_nodes ADD COLUMN grpc_token TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE server_nodes ADD COLUMN xray_state TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE server_nodes ADD COLUMN xray_version TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE server_nodes ADD COLUMN relay_version TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE server_nodes ADD COLUMN wg_backend TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE server_nodes ADD COLUMN xui_node_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE server_nodes ADD COLUMN xui_inbound_tag TEXT NOT NULL DEFAULT ''`,
	} {
		if _, err := db.Exec(alter); err != nil {
			if !strings.Contains(err.Error(), "duplicate column name") {
				return fmt.Errorf("storage: %s: %w", alter, err)
			}
		}
	}
	// Инвайты, погашенные до появления счётчика, надо отметить потраченными.
	// Иначе миграция воскрешает их: use_count у них 0, max_uses по умолчанию 1,
	// и код, которым уже воспользовались, снова становится годным.
	if _, err := db.Exec(`UPDATE invite_tokens SET use_count = 1 WHERE used_at > 0 AND use_count = 0`); err != nil {
		return fmt.Errorf("storage: backfill invite use_count: %w", err)
	}
	return nil
}

// migrateAdminUsernamesToLower lowercases every existing admin username so the
// case-insensitive login flow lines up with the table. Runs on every startup
// but only touches rows that actually differ, so it is a no-op afterward.
// Collisions (two admins differing only in case) abort with a clear error
// rather than silently merging rows.
func migrateAdminUsernamesToLower(gdb *gorm.DB) error {
	type row struct {
		ID       int64
		Username string
	}
	var rows []row
	if err := gdb.Raw(`SELECT id, username FROM admins WHERE username <> lower(username)`).Scan(&rows).Error; err != nil {
		return err
	}
	for _, r := range rows {
		lowered := strings.ToLower(r.Username)
		var collidingID int64
		err := gdb.Raw(
			`SELECT id FROM admins WHERE lower(username) = ? AND id <> ?`,
			lowered, r.ID,
		).Scan(&collidingID).Error
		if err != nil {
			return err
		}
		if collidingID != 0 {
			return fmt.Errorf(
				"username collision when lowercasing admin id=%d %q: row id=%d already owns %q",
				r.ID, r.Username, collidingID, lowered,
			)
		}
		if err := gdb.Exec(`UPDATE admins SET username = ? WHERE id = ?`, lowered, r.ID).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// DB returns the underlying database/sql handle used by the raw-SQL methods
// still being ported to GORM.
func (s *Store) DB() *sql.DB {
	return s.db
}

// Gorm returns the GORM handle for ported methods and the db-migrate command.
func (s *Store) Gorm() *gorm.DB {
	return s.gdb
}

// migrateNodeWGBackends gives every vk-turn-proxy node an explicit wg backend.
//
// An empty wg_backend used to mean "fall through to the panel-global XUI_WG_* env",
// an invisible default that overrode the per-node model and pointed provisioning at
// whatever node that env named - on this panel, at a node id that no longer existed,
// so provisioning failed with "storage: not found" and the UI still labelled the
// empty value as the node's own wg. The env is read here one final time so a
// self-hosted panel keeps its current behaviour across the upgrade; after this runs
// nothing consults it again.
func migrateNodeWGBackends(gdb *gorm.DB) error {
	type row struct{ ID string }
	var rows []row
	err := gdb.Raw(
		`SELECT id FROM server_nodes WHERE kind = ? AND (wg_backend IS NULL OR wg_backend = '')`,
		ServerNodeVKTurnProxy,
	).Scan(&rows).Error
	if err != nil || len(rows) == 0 {
		return err
	}
	legacyNode := strings.TrimSpace(os.Getenv("XUI_WG_NODE_ID"))
	legacyTag := strings.TrimSpace(os.Getenv("XUI_WG_INBOUND_TAG"))
	backend, xuiNode, xuiTag := WGBackendOwn, "", ""
	if legacyNode != "" {
		var exists int64
		if err := gdb.Raw(`SELECT count(1) FROM server_nodes WHERE id = ?`, legacyNode).Scan(&exists).Error; err != nil {
			return err
		}
		// A legacy env naming a node that is gone describes no working setup, so
		// those nodes are better off owning their wg than inheriting a dead target.
		if exists > 0 {
			backend, xuiNode, xuiTag = WGBackendXUI, legacyNode, legacyTag
		}
	}
	for _, r := range rows {
		err := gdb.Exec(
			`UPDATE server_nodes SET wg_backend = ?, xui_node_id = ?, xui_inbound_tag = ? WHERE id = ?`,
			backend, xuiNode, xuiTag, r.ID,
		).Error
		if err != nil {
			return err
		}
	}
	log.Printf("storage: gave %d vk-turn node(s) an explicit wg backend (%s)", len(rows), backend)
	return nil
}
