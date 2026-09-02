package storage

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"v.wingsnet.org/internal/avatarpic"
	"v.wingsnet.org/internal/storage/dbmodel"
)

const (
	RoleOwner = "owner"
	RoleAdmin = "admin"
)

type Admin struct {
	ID                 int64
	Username           string
	PasswordHash       string
	MustChangePassword bool
	Role               string
	// PanelAccess - открыта ли админ-панель. Личный доступ к VPN есть у любого
	// аккаунта, панель к нему добавляется отдельно
	PanelAccess bool
	// PanelRequestedAt - когда попросил панель. Нулевое время - не просил
	PanelRequestedAt time.Time
	LastLoginAt      time.Time
	AvatarVersion    int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

var ErrNotFound = errors.New("storage: not found")

// toStorageAdmin maps the gorm row model to the app-facing Admin.
func toStorageAdmin(m dbmodel.Admin) Admin {
	a := Admin{
		ID:                 m.ID,
		Username:           m.Username,
		PasswordHash:       m.PasswordHash,
		MustChangePassword: m.MustChangePassword != 0,
		Role:               m.Role,
		PanelAccess:        m.PanelAccess != 0,
		LastLoginAt:        time.UnixMilli(m.LastLoginAt).UTC(),
		AvatarVersion:      m.AvatarVersion,
		CreatedAt:          time.UnixMilli(m.CreatedAtUnix).UTC(),
		UpdatedAt:          time.UnixMilli(m.UpdatedAtUnix).UTC(),
	}
	if m.PanelRequestedAt > 0 {
		a.PanelRequestedAt = time.UnixMilli(m.PanelRequestedAt).UTC()
	}
	if a.Role == "" {
		a.Role = RoleAdmin
	}
	return a
}

// RequestPanelAccess отмечает, что участник просит открыть ему панель. Повторный
// вызов не двигает отметку: очередь заявок должна идти по времени первой просьбы
func (s *Store) RequestPanelAccess(id int64) error {
	return s.gdb.Model(&dbmodel.Admin{}).
		Where("id = ? AND panel_requested_at = 0", id).
		Updates(map[string]any{
			"panel_requested_at": time.Now().UTC().UnixMilli(),
			"updated_at":         time.Now().UTC().UnixMilli(),
		}).Error
}

// ClearPanelRequest снимает заявку - решение по ней принято
func (s *Store) ClearPanelRequest(id int64) error {
	return s.gdb.Model(&dbmodel.Admin{}).Where("id = ?", id).
		Updates(map[string]any{
			"panel_requested_at": 0,
			"updated_at":         time.Now().UTC().UnixMilli(),
		}).Error
}

func (s *Store) CreateAdmin(username, passwordHash string, mustChange bool, role string) (Admin, error) {
	return s.CreateAccount(username, passwordHash, mustChange, role, true)
}

// CreateAccount заводит аккаунт с явно указанным доступом в панель. Пришедший по
// приглашению получает личный доступ к VPN, но не панель: её выдаёт владелец
func (s *Store) CreateAccount(username, passwordHash string, mustChange bool, role string, panelAccess bool) (Admin, error) {
	if role == "" {
		role = RoleAdmin
	}
	now := time.Now().UTC().UnixMilli()
	row := dbmodel.Admin{
		Username:           username,
		PasswordHash:       passwordHash,
		MustChangePassword: int64(boolToInt(mustChange)),
		Role:               role,
		PanelAccess:        int64(boolToInt(panelAccess)),
		CreatedAtUnix:      now,
		UpdatedAtUnix:      now,
	}
	if err := s.gdb.Create(&row).Error; err != nil {
		return Admin{}, err
	}
	// Аватар заводится вместе с аккаунтом: он есть с первой же минуты, а не
	// появляется хуй знает когда
	if picture := avatarpic.Default(); len(picture) > 0 {
		if _, err := s.SetAdminAvatar(row.ID, "image/png", picture); err == nil {
			row.AvatarVersion++
		}
	}
	return toStorageAdmin(row), nil
}

func (s *Store) FindAdminByUsername(username string) (Admin, error) {
	return s.findAdmin("username = ?", username)
}

func (s *Store) FindAdminByID(id int64) (Admin, error) {
	return s.findAdmin("id = ?", id)
}

func (s *Store) findAdmin(query string, arg any) (Admin, error) {
	var m dbmodel.Admin
	err := s.gdb.Where(query, arg).First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Admin{}, ErrNotFound
	}
	if err != nil {
		return Admin{}, err
	}
	return toStorageAdmin(m), nil
}

func (s *Store) ListAdmins() ([]Admin, error) {
	var ms []dbmodel.Admin
	if err := s.gdb.Order("created_at ASC").Find(&ms).Error; err != nil {
		return nil, err
	}
	out := make([]Admin, 0, len(ms))
	for _, m := range ms {
		out = append(out, toStorageAdmin(m))
	}
	return out, nil
}

func (s *Store) UpdateAdminPassword(id int64, passwordHash string, requireChange bool) error {
	return s.gdb.Model(&dbmodel.Admin{}).Where("id = ?", id).Updates(map[string]any{
		"password_hash":        passwordHash,
		"must_change_password": boolToInt(requireChange),
		"updated_at":           time.Now().UTC().UnixMilli(),
	}).Error
}

func (s *Store) UpdateAdminRole(id int64, role string) error {
	return s.gdb.Model(&dbmodel.Admin{}).Where("id = ?", id).Updates(map[string]any{
		"role":       role,
		"updated_at": time.Now().UTC().UnixMilli(),
	}).Error
}

func (s *Store) MarkAdminLogin(id int64) error {
	return s.gdb.Model(&dbmodel.Admin{}).Where("id = ?", id).
		Update("last_login_at", time.Now().UTC().UnixMilli()).Error
}

func (s *Store) DeleteAdmin(id int64) error {
	res := s.gdb.Where("id = ?", id).Delete(&dbmodel.Admin{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CountAdmins() (int, error) {
	var count int64
	if err := s.gdb.Model(&dbmodel.Admin{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return int(count), nil
}

func (s *Store) FirstAdminID() (int64, error) {
	var id int64
	if err := s.gdb.Model(&dbmodel.Admin{}).Select("COALESCE(MIN(id),0)").Scan(&id).Error; err != nil {
		return 0, err
	}
	if id == 0 {
		return 0, ErrNotFound
	}
	return id, nil
}

// EnsureAtLeastOneOwner promotes the lowest-id admin to "owner" if no owner
// currently exists. Used on startup to migrate pre-role databases.
func (s *Store) EnsureAtLeastOneOwner() error {
	var count int64
	if err := s.gdb.Model(&dbmodel.Admin{}).Where("role = ?", RoleOwner).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	id, err := s.FirstAdminID()
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil
		}
		return err
	}
	return s.UpdateAdminRole(id, RoleOwner)
}

// SetAdminAvatar stores the avatar bytes + mime, bumping the version so cached
// URLs invalidate.
//
// Сами байты уезжают в blobs под своим хешем: одинаковую картинку грузит не один
// человек, и плодить её копию на каждого - тупая трата места нахуй
func (s *Store) SetAdminAvatar(id int64, mime string, bytes []byte) (int64, error) {
	hash, err := s.PutBlob(mime, bytes)
	if err != nil {
		return 0, err
	}
	previous := s.avatarBlobOf(id)
	if err := s.gdb.Model(&dbmodel.Admin{}).Where("id = ?", id).Updates(map[string]any{
		"avatar_mime":    mime,
		"avatar_blob":    hash,
		"avatar_png":     gorm.Expr("NULL"),
		"avatar_version": gorm.Expr("avatar_version + 1"),
		"updated_at":     time.Now().UTC().UnixMilli(),
	}).Error; err != nil {
		return 0, err
	}
	if previous != hash {
		s.dropBlobIfOrphan(previous)
	}
	var m dbmodel.Admin
	if err := s.gdb.Select("avatar_version").Where("id = ?", id).First(&m).Error; err != nil {
		return 0, err
	}
	return m.AvatarVersion, nil
}

// GetAdminAvatar returns the stored avatar bytes and mime. Empty bytes mean
// no custom avatar was uploaded (frontend falls back to default).
func (s *Store) GetAdminAvatar(id int64) (mime string, data []byte, version int64, err error) {
	var m dbmodel.Admin
	if err = s.gdb.Select("avatar_mime", "avatar_png", "avatar_blob", "avatar_version").
		Where("id = ?", id).First(&m).Error; err != nil {
		return "", nil, 0, err
	}
	if m.AvatarBlob != "" {
		blobMime, blobData, blobErr := s.GetBlob(m.AvatarBlob)
		if blobErr == nil {
			if blobMime == "" {
				blobMime = m.AvatarMime
			}
			return blobMime, blobData, m.AvatarVersion, nil
		}
	}
	// Старьё, заведённое до blobs, держит байты прямо в себе - его не бросаем
	return m.AvatarMime, m.AvatarPNG, m.AvatarVersion, nil
}

// avatarBlobOf - хеш текущей картинки аккаунта
func (s *Store) avatarBlobOf(id int64) string {
	var m dbmodel.Admin
	if err := s.gdb.Select("avatar_blob").Where("id = ?", id).First(&m).Error; err != nil {
		return ""
	}
	return m.AvatarBlob
}

// ClearAdminAvatar wipes the avatar. Version still bumps so caches refresh
// to the default image.
func (s *Store) ClearAdminAvatar(id int64) error {
	previous := s.avatarBlobOf(id)
	if err := s.gdb.Model(&dbmodel.Admin{}).Where("id = ?", id).Updates(map[string]any{
		"avatar_mime":    "",
		"avatar_png":     gorm.Expr("NULL"),
		"avatar_blob":    "",
		"avatar_version": gorm.Expr("avatar_version + 1"),
		"updated_at":     time.Now().UTC().UnixMilli(),
	}).Error; err != nil {
		return err
	}
	s.dropBlobIfOrphan(previous)
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// SetPanelAccess открывает или закрывает аккаунту админ-панель
func (s *Store) SetPanelAccess(id int64, allowed bool) error {
	return s.gdb.Model(&dbmodel.Admin{}).Where("id = ?", id).
		Updates(map[string]any{
			"panel_access": boolToInt(allowed),
			"updated_at":   time.Now().UTC().UnixMilli(),
		}).Error
}
