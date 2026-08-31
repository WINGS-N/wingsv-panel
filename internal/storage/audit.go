package storage

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"v.wingsnet.org/internal/storage/dbmodel"
)

const (
	SettingRegistrationMode = "registration_mode"
	// SettingAllowAdminGRPC gates whether non-owner admins may register their own
	// external vk-turn-proxy / 3x-ui gRPC endpoints. Stored as "true"/"false".
	SettingAllowAdminGRPC = "allow_admin_grpc"
	// SettingFederationEnabled gates every federation surface. Off by default,
	// and the sections behind it are not rendered at all rather than rendered
	// disabled: a deployment that never opted in has nothing to reason about.
	SettingFederationEnabled = "federation_enabled"

	// SettingLinkFormat выбирает, чем сжимать ссылки: brotli короче на четверть,
	// но сборки приложения до его появления такую ссылку не прочитают
	SettingLinkFormat = "link_format"
	// LinkFormatBrotli и LinkFormatDeflate - значения этой настройки
	LinkFormatBrotli  = "brotli"
	LinkFormatDeflate = "deflate"
)

type AuditEntry struct {
	ID            int64
	TS            time.Time
	ActorAdminID  int64
	ActorUsername string
	Action        string
	TargetType    string
	TargetID      string
	Message       string
	IP            string
}

type AuditFilter struct {
	ActorAdminID int64
	Action       string
	Since        time.Time
	Until        time.Time
	Limit        int
	Offset       int
}

func (s *Store) AppendAudit(entry AuditEntry) error {
	if entry.Action == "" {
		return errors.New("storage: audit action required")
	}
	ts := time.Now().UTC().UnixMilli()
	if !entry.TS.IsZero() {
		ts = entry.TS.UTC().UnixMilli()
	}
	var actor *int64
	if entry.ActorAdminID > 0 {
		a := entry.ActorAdminID
		actor = &a
	}
	return s.gdb.Create(&dbmodel.AuditLog{
		Ts:            ts,
		ActorAdminID:  actor,
		ActorUsername: entry.ActorUsername,
		Action:        entry.Action,
		TargetType:    entry.TargetType,
		TargetID:      entry.TargetID,
		Message:       entry.Message,
		IP:            entry.IP,
	}).Error
}

// derefInt64 returns the pointed-to value, or 0 for a nil pointer - the gorm
// equivalent of the COALESCE(col, 0) the raw reads used for nullable id columns.
func derefInt64(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

// CountAudit - сколько записей под фильтр: без общего числа пагинация не знает,
// сколько всего страниц
func (s *Store) CountAudit(filter AuditFilter) (int64, error) {
	var count int64
	err := s.auditQuery(filter).Count(&count).Error
	return count, err
}

func (s *Store) auditQuery(filter AuditFilter) *gorm.DB {
	q := s.gdb.Model(&dbmodel.AuditLog{})
	if filter.ActorAdminID > 0 {
		q = q.Where("actor_admin_id = ?", filter.ActorAdminID)
	}
	if filter.Action != "" {
		q = q.Where("action = ?", filter.Action)
	}
	if !filter.Since.IsZero() {
		q = q.Where("ts >= ?", filter.Since.UTC().UnixMilli())
	}
	if !filter.Until.IsZero() {
		q = q.Where("ts <= ?", filter.Until.UTC().UnixMilli())
	}
	return q
}

func (s *Store) ListAudit(filter AuditFilter) ([]AuditEntry, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := s.auditQuery(filter)
	var rows []dbmodel.AuditLog
	if err := q.Order("ts DESC").Limit(limit).Offset(filter.Offset).Find(&rows).Error; err != nil {
		return nil, err
	}
	var out []AuditEntry
	for _, r := range rows {
		out = append(out, AuditEntry{
			ID:            r.ID,
			TS:            time.UnixMilli(r.Ts).UTC(),
			ActorAdminID:  derefInt64(r.ActorAdminID),
			ActorUsername: r.ActorUsername,
			Action:        r.Action,
			TargetType:    r.TargetType,
			TargetID:      r.TargetID,
			Message:       r.Message,
			IP:            r.IP,
		})
	}
	return out, nil
}

// PruneAuditOlderThan deletes entries older than `cutoff`. Caller decides cadence.
func (s *Store) PruneAuditOlderThan(cutoff time.Time) error {
	return s.gdb.Where("ts < ?", cutoff.UTC().UnixMilli()).Delete(&dbmodel.AuditLog{}).Error
}

// ===== platform_settings =====

func (s *Store) GetPlatformSetting(key, fallback string) (string, error) {
	var row dbmodel.PlatformSetting
	err := s.gdb.Where(&dbmodel.PlatformSetting{Key: key}).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return fallback, nil
	}
	if err != nil {
		return "", err
	}
	return row.Value, nil
}

func (s *Store) SetPlatformSetting(key, value string) error {
	return s.gdb.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value"}),
	}).Create(&dbmodel.PlatformSetting{Key: key, Value: value}).Error
}

// ===== invite_tokens =====

type InviteToken struct {
	Token            string
	CreatedAt        time.Time
	ExpiresAt        time.Time
	UsedAt           time.Time
	UsedByAdminID    int64
	CreatedByAdminID int64
	// MaxUses - сколько человек может зарегистрироваться по коду, UseCount -
	// сколько уже зарегистрировалось
	MaxUses  int64
	UseCount int64
}

func (s *Store) CreateInvite(token string, expiresAt time.Time, createdByAdminID int64) (InviteToken, error) {
	return s.CreateInviteWithUses(token, expiresAt, createdByAdminID, 1)
}

// CreateInviteWithUses issues a code several people may register on.
//
// The limit is not recursive: it caps who joins on this code, not how many
// people they go on to invite with codes of their own.
func (s *Store) CreateInviteWithUses(token string, expiresAt time.Time, createdByAdminID int64, maxUses int64) (InviteToken, error) {
	if maxUses < 1 {
		maxUses = 1
	}
	now := time.Now().UTC().UnixMilli()
	exp := int64(0)
	if !expiresAt.IsZero() {
		exp = expiresAt.UTC().UnixMilli()
	}
	createdBy := createdByAdminID
	if err := s.gdb.Create(&dbmodel.InviteToken{
		Token:            token,
		CreatedAtUnix:    now,
		ExpiresAt:        exp,
		CreatedByAdminID: &createdBy,
		MaxUses:          maxUses,
	}).Error; err != nil {
		return InviteToken{}, err
	}
	return InviteToken{
		Token:            token,
		CreatedAt:        time.UnixMilli(now).UTC(),
		ExpiresAt:        time.UnixMilli(exp).UTC(),
		CreatedByAdminID: createdByAdminID,
		MaxUses:          maxUses,
	}, nil
}

// ListInvitesByAdmin returns the codes one administrator handed out. Everyone
// grows their own branch of the tree, so everyone sees their own codes and
// nobody else's.
func (s *Store) ListInvitesByAdmin(adminID int64) ([]InviteToken, error) {
	var rows []dbmodel.InviteToken
	if err := s.gdb.Where("created_by_admin_id = ?", adminID).
		Order("created_at DESC").Limit(200).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]InviteToken, 0, len(rows))
	for _, r := range rows {
		out = append(out, InviteToken{
			Token:            r.Token,
			CreatedAt:        time.UnixMilli(r.CreatedAtUnix).UTC(),
			ExpiresAt:        time.UnixMilli(r.ExpiresAt).UTC(),
			UsedAt:           time.UnixMilli(r.UsedAt).UTC(),
			UsedByAdminID:    derefInt64(r.UsedByAdminID),
			CreatedByAdminID: derefInt64(r.CreatedByAdminID),
			MaxUses:          r.MaxUses,
			UseCount:         r.UseCount,
		})
	}
	return out, nil
}

func (s *Store) ListInvites(includeUsed bool) ([]InviteToken, error) {
	q := s.gdb.Model(&dbmodel.InviteToken{})
	if !includeUsed {
		q = q.Where("used_at = ?", 0)
	}
	var rows []dbmodel.InviteToken
	if err := q.Order("created_at DESC").Limit(200).Find(&rows).Error; err != nil {
		return nil, err
	}
	var out []InviteToken
	for _, r := range rows {
		out = append(out, InviteToken{
			Token:            r.Token,
			CreatedAt:        time.UnixMilli(r.CreatedAtUnix).UTC(),
			ExpiresAt:        time.UnixMilli(r.ExpiresAt).UTC(),
			UsedAt:           time.UnixMilli(r.UsedAt).UTC(),
			UsedByAdminID:    derefInt64(r.UsedByAdminID),
			CreatedByAdminID: derefInt64(r.CreatedByAdminID),
			MaxUses:          r.MaxUses,
			UseCount:         r.UseCount,
		})
	}
	return out, nil
}

func (s *Store) DeleteInvite(token string) error {
	return s.gdb.Where("token = ?", token).Delete(&dbmodel.InviteToken{}).Error
}

// RedeemInvite spends one use of the token. Returns ErrNotFound if the token
// does not exist, has expired, or has no uses left.
//
// The check and the increment are one statement on purpose: two people
// registering on the last use of a group code at the same moment would both
// pass a separate read, and the code would be spent twice.
func (s *Store) RedeemInvite(token string, adminID int64) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return ErrNotFound
	}
	now := time.Now().UTC().UnixMilli()
	res := s.gdb.Model(&dbmodel.InviteToken{}).
		Where("token = ? AND use_count < max_uses AND (expires_at = 0 OR expires_at > ?)", token, now).
		Updates(map[string]any{
			"use_count": gorm.Expr("use_count + 1"),
			// UsedAt - время первого погашения, дальше не трогаем
			"used_at":          gorm.Expr("CASE WHEN used_at = 0 THEN ? ELSE used_at END", now),
			"used_by_admin_id": gorm.Expr("COALESCE(used_by_admin_id, ?)", adminID),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrNotFound
	}
	// Ребро дерева пишем отдельно: у многоразового кода приглашённых несколько,
	// а поле в самом коде вмещает одного. Провал здесь означает списанный код
	// без места в дереве, поэтому он возвращается наверх
	return s.gdb.Create(&dbmodel.InviteRedemption{
		Token: token, AdminID: adminID, CreatedAtUnix: now,
	}).Error
}

// RedeemedInvite сообщает, пришёл ли аккаунт по приглашению
func (s *Store) RedeemedInvite(adminID int64) (bool, error) {
	var count int64
	err := s.gdb.Model(&dbmodel.InviteRedemption{}).Where("admin_id = ?", adminID).Count(&count).Error
	if err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}
	// Коды, погашенные до появления таблицы
	err = s.gdb.Model(&dbmodel.InviteToken{}).Where("used_by_admin_id = ?", adminID).Count(&count).Error
	return count > 0, err
}

// FindInvite reads one code. Used by the invite page, which has no session -
// hence it returns the row and lets the caller decide what to reveal.
func (s *Store) FindInvite(token string) (InviteToken, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return InviteToken{}, ErrNotFound
	}
	var r dbmodel.InviteToken
	if err := s.gdb.First(&r, "token = ?", token).Error; err != nil {
		return InviteToken{}, ErrNotFound
	}
	return InviteToken{
		Token:            r.Token,
		CreatedAt:        time.UnixMilli(r.CreatedAtUnix).UTC(),
		ExpiresAt:        time.UnixMilli(r.ExpiresAt).UTC(),
		UsedAt:           time.UnixMilli(r.UsedAt).UTC(),
		UsedByAdminID:    derefInt64(r.UsedByAdminID),
		CreatedByAdminID: derefInt64(r.CreatedByAdminID),
		MaxUses:          r.MaxUses,
		UseCount:         r.UseCount,
	}, nil
}
