package storage

import (
	"database/sql"
	"errors"
)

func (s *Store) KVGet(key string) ([]byte, error) {
	row := s.db.QueryRow(`SELECT value FROM kv WHERE key = ?`, key)
	var v []byte
	err := row.Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return v, nil
}

func (s *Store) KVSet(key string, value []byte) error {
	_, err := s.db.Exec(`
		INSERT INTO kv (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value)
	return err
}
