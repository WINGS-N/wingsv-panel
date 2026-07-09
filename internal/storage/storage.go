package storage

import (
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"strconv"
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

// rebind rewrites `?` placeholders to `$1, $2, ...` for PostgreSQL; sqlite and
// mariadb use `?` natively so the query is returned unchanged. The raw queries
// never contain a literal `?`, so a positional counter is sufficient.
func (s *Store) rebind(query string) string {
	if s.driver != DriverPostgres {
		return query
	}
	var b strings.Builder
	b.Grow(len(query) + 8)
	n := 0
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteByte(query[i])
	}
	return b.String()
}

func (s *Store) query(q string, args ...any) (*sql.Rows, error) {
	return s.db.Query(s.rebind(q), args...)
}

func (s *Store) queryRow(q string, args ...any) *sql.Row {
	return s.db.QueryRow(s.rebind(q), args...)
}

func (s *Store) exec(q string, args ...any) (sql.Result, error) {
	return s.db.Exec(s.rebind(q), args...)
}

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
	return &Store{gdb: gdb, db: sqlDB, driver: driver}, nil
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
