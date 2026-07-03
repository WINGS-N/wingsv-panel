package storage

import (
	"database/sql"
	"errors"

	"gorm.io/gorm/clause"

	"v.wingsnet.org/internal/storage/dbmodel"
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
	return s.gdb.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value"}),
	}).Create(&dbmodel.KV{Key: key, Value: value}).Error
}
