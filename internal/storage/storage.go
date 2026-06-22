package storage

import (
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

type Store struct {
	db *sql.DB
}

func Open(dbPath string) (*Store, error) {
	if dbPath == "" {
		return nil, errors.New("storage: empty db path")
	}
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=synchronous(NORMAL)", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("storage: open: %w", err)
	}
	// WAL allows many concurrent readers alongside a single writer, so a single
	// pooled connection (the old SetMaxOpenConns(1)) is needlessly serializing
	// the whole panel: one guardian welcome flow or a reconnect storm could
	// starve every admin request. Allow real read concurrency; writes still
	// serialize at the SQLite level and wait out contention via busy_timeout.
	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(16)
	db.SetConnMaxIdleTime(5 * time.Minute)
	if _, err := db.Exec(schemaSQL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("storage: apply schema: %w", err)
	}
	// Idempotent column-adds for upgrades from older schemas. SQLite returns
	// "duplicate column name" when the column already exists; that's expected
	// and ignored.
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
	} {
		if _, err := db.Exec(alter); err != nil {
			if !strings.Contains(err.Error(), "duplicate column name") {
				_ = db.Close()
				return nil, fmt.Errorf("storage: %s: %w", alter, err)
			}
		}
	}
	if err := migrateAdminUsernamesToLower(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("storage: migrate admin usernames: %w", err)
	}
	return &Store{db: db}, nil
}

// migrateAdminUsernamesToLower lowercases every existing admin username so the
// case-insensitive login flow (auth.NormalizeUsername) lines up with what's in
// the table. Run on every startup but only touches rows that actually differ,
// so it's a no-op after the first migration. Collisions (two admins differing
// only in case) abort the migration and surface a clear error rather than
// silently merging the rows.
func migrateAdminUsernamesToLower(db *sql.DB) error {
	rows, err := db.Query(`SELECT id, username FROM admins WHERE username <> lower(username)`)
	if err != nil {
		return err
	}
	type pending struct {
		id       int64
		lowered  string
		original string
	}
	var todo []pending
	for rows.Next() {
		var p pending
		if err := rows.Scan(&p.id, &p.original); err != nil {
			rows.Close()
			return err
		}
		p.lowered = strings.ToLower(p.original)
		todo = append(todo, p)
	}
	rows.Close()
	if len(todo) == 0 {
		return nil
	}
	for _, p := range todo {
		var collidingID int64
		err := db.QueryRow(
			`SELECT id FROM admins WHERE lower(username) = ? AND id <> ?`,
			p.lowered, p.id,
		).Scan(&collidingID)
		if err == nil {
			return fmt.Errorf(
				"username collision when lowercasing admin id=%d %q: row id=%d already owns %q",
				p.id, p.original, collidingID, p.lowered,
			)
		} else if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err := db.Exec(`UPDATE admins SET username = ? WHERE id = ?`, p.lowered, p.id); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) DB() *sql.DB {
	return s.db
}
