package storage

import (
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"strings"

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
	db.SetMaxOpenConns(1)
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
	} {
		if _, err := db.Exec(alter); err != nil {
			if !strings.Contains(err.Error(), "duplicate column name") {
				_ = db.Close()
				return nil, fmt.Errorf("storage: %s: %w", alter, err)
			}
		}
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) DB() *sql.DB {
	return s.db
}
