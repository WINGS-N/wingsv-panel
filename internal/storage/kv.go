package storage

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"v.wingsnet.org/internal/storage/dbmodel"
)

func (s *Store) KVGet(key string) ([]byte, error) {
	var row dbmodel.KV
	err := s.gdb.Where(&dbmodel.KV{Key: key}).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return row.Value, nil
}

func (s *Store) KVSet(key string, value []byte) error {
	return s.gdb.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value"}),
	}).Create(&dbmodel.KV{Key: key, Value: value}).Error
}
