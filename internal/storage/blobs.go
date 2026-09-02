package storage

import (
	"crypto/sha512"
	"encoding/hex"
	"time"

	"gorm.io/gorm"

	"v.wingsnet.org/internal/storage/dbmodel"
)

// BlobHash - как считается ключ содержимого.
//
// SHA-512/256: SHA-256 в проекте не используем вообще, а этот на 64-битных ещё и
// быстрее, и на length-extension его не наебёшь. Ключ он же и дедуп: одинаковые
// байты дают один хеш, и второй копии в базе не заводится
func BlobHash(data []byte) string {
	sum := sha512.Sum512_256(data)
	return hex.EncodeToString(sum[:])
}

// PutBlob кладёт содержимое и возвращает его хеш.
//
// Если такие байты уже валяются, нихуя не пишется: десять аккаунтов с одинаковой
// аватаркой держат одну строку на всех, а не десять копий
func (s *Store) PutBlob(mime string, data []byte) (string, error) {
	if len(data) == 0 {
		return "", ErrNotFound
	}
	hash := BlobHash(data)
	var existing dbmodel.Blob
	if err := s.gdb.Select("hash").First(&existing, "hash = ?", hash).Error; err == nil {
		return hash, nil
	}
	row := dbmodel.Blob{
		Hash:          hash,
		Mime:          mime,
		Data:          data,
		Size:          int64(len(data)),
		CreatedAtUnix: time.Now().UTC().UnixMilli(),
	}
	if err := s.gdb.Create(&row).Error; err != nil {
		// Две одинаковые загрузки столкнулись лбами: строку успел записать сосед,
		// и это ровно то, чего мы и добивались
		var check dbmodel.Blob
		if lookup := s.gdb.Select("hash").First(&check, "hash = ?", hash).Error; lookup == nil {
			return hash, nil
		}
		return "", err
	}
	return hash, nil
}

// GetBlob отдаёт содержимое по хешу
func (s *Store) GetBlob(hash string) (string, []byte, error) {
	if hash == "" {
		return "", nil, ErrNotFound
	}
	var row dbmodel.Blob
	if err := s.gdb.First(&row, "hash = ?", hash).Error; err != nil {
		return "", nil, ErrNotFound
	}
	return row.Mime, row.Data, nil
}

// dropBlobIfOrphan выкидывает содержимое, на которое уже никто не ссылается.
//
// Без этого база копит картинки дохлых аккаунтов, а место они жрут настоящее
func (s *Store) dropBlobIfOrphan(hash string) {
	if hash == "" {
		return
	}
	var refs int64
	if err := s.gdb.Model(&dbmodel.Admin{}).Where("avatar_blob = ?", hash).Count(&refs).Error; err != nil {
		return
	}
	if refs > 0 {
		return
	}
	_ = s.gdb.Where("hash = ?", hash).Delete(&dbmodel.Blob{}).Error
}

var _ = gorm.Expr
