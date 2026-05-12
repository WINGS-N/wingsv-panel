package storage

import (
	"database/sql"
	"errors"
	"time"
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
	LastLoginAt        time.Time
	AvatarVersion      int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

var ErrNotFound = errors.New("storage: not found")

func (s *Store) CreateAdmin(username, passwordHash string, mustChange bool, role string) (Admin, error) {
	if role == "" {
		role = RoleAdmin
	}
	now := time.Now().UTC().UnixMilli()
	res, err := s.db.Exec(
		`INSERT INTO admins (username, password_hash, must_change_password, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		username, passwordHash, boolToInt(mustChange), role, now, now,
	)
	if err != nil {
		return Admin{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Admin{}, err
	}
	return Admin{
		ID:                 id,
		Username:           username,
		PasswordHash:       passwordHash,
		MustChangePassword: mustChange,
		Role:               role,
		CreatedAt:          time.UnixMilli(now).UTC(),
		UpdatedAt:          time.UnixMilli(now).UTC(),
	}, nil
}

const adminColumns = `id, username, password_hash, must_change_password, role, last_login_at, avatar_version, created_at, updated_at`

func (s *Store) FindAdminByUsername(username string) (Admin, error) {
	row := s.db.QueryRow(`SELECT `+adminColumns+` FROM admins WHERE username = ?`, username)
	return scanAdmin(row)
}

func (s *Store) FindAdminByID(id int64) (Admin, error) {
	row := s.db.QueryRow(`SELECT `+adminColumns+` FROM admins WHERE id = ?`, id)
	return scanAdmin(row)
}

func (s *Store) ListAdmins() ([]Admin, error) {
	rows, err := s.db.Query(`SELECT ` + adminColumns + ` FROM admins ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Admin
	for rows.Next() {
		a, err := scanAdminRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) UpdateAdminPassword(id int64, passwordHash string, requireChange bool) error {
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`UPDATE admins SET password_hash = ?, must_change_password = ?, updated_at = ? WHERE id = ?`,
		passwordHash, boolToInt(requireChange), now, id,
	)
	return err
}

func (s *Store) UpdateAdminRole(id int64, role string) error {
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(`UPDATE admins SET role = ?, updated_at = ? WHERE id = ?`, role, now, id)
	return err
}

func (s *Store) MarkAdminLogin(id int64) error {
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(`UPDATE admins SET last_login_at = ? WHERE id = ?`, now, id)
	return err
}

func (s *Store) DeleteAdmin(id int64) error {
	res, err := s.db.Exec(`DELETE FROM admins WHERE id = ?`, id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) CountAdmins() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(1) FROM admins`).Scan(&count)
	return count, err
}

func (s *Store) FirstAdminID() (int64, error) {
	var id int64
	err := s.db.QueryRow(`SELECT MIN(id) FROM admins`).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}

// EnsureAtLeastOneOwner promotes the lowest-id admin to "owner" if no owner
// currently exists. Used on startup to migrate pre-role databases.
func (s *Store) EnsureAtLeastOneOwner() error {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(1) FROM admins WHERE role = ?`, RoleOwner).Scan(&count); err != nil {
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

func scanAdmin(row *sql.Row) (Admin, error) {
	a, err := scanAdminFromScanner(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Admin{}, ErrNotFound
	}
	return a, err
}

func scanAdminRow(rows *sql.Rows) (Admin, error) {
	return scanAdminFromScanner(rows)
}

func scanAdminFromScanner(scanner rowScanner) (Admin, error) {
	var a Admin
	var must int
	var lastLogin, createdAt, updatedAt int64
	err := scanner.Scan(&a.ID, &a.Username, &a.PasswordHash, &must, &a.Role, &lastLogin, &a.AvatarVersion, &createdAt, &updatedAt)
	if err != nil {
		return Admin{}, err
	}
	a.MustChangePassword = must != 0
	a.LastLoginAt = time.UnixMilli(lastLogin).UTC()
	a.CreatedAt = time.UnixMilli(createdAt).UTC()
	a.UpdatedAt = time.UnixMilli(updatedAt).UTC()
	if a.Role == "" {
		a.Role = RoleAdmin
	}
	return a, nil
}

// SetAdminAvatar stores the avatar bytes + mime, bumping the version so cached
// URLs invalidate.
func (s *Store) SetAdminAvatar(id int64, mime string, bytes []byte) (int64, error) {
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`UPDATE admins
		 SET avatar_mime = ?, avatar_png = ?, avatar_version = avatar_version + 1, updated_at = ?
		 WHERE id = ?`,
		mime, bytes, now, id,
	)
	if err != nil {
		return 0, err
	}
	var version int64
	if err := s.db.QueryRow(`SELECT avatar_version FROM admins WHERE id = ?`, id).Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}

// GetAdminAvatar returns the stored avatar bytes and mime. Empty bytes mean
// no custom avatar was uploaded (frontend falls back to default).
func (s *Store) GetAdminAvatar(id int64) (mime string, data []byte, version int64, err error) {
	row := s.db.QueryRow(`SELECT COALESCE(avatar_mime, ''), avatar_png, avatar_version FROM admins WHERE id = ?`, id)
	if err = row.Scan(&mime, &data, &version); err != nil {
		return "", nil, 0, err
	}
	return mime, data, version, nil
}

// ClearAdminAvatar wipes the avatar. Version still bumps so caches refresh
// to the default image.
func (s *Store) ClearAdminAvatar(id int64) error {
	now := time.Now().UTC().UnixMilli()
	_, err := s.db.Exec(
		`UPDATE admins SET avatar_mime = '', avatar_png = NULL,
		 avatar_version = avatar_version + 1, updated_at = ? WHERE id = ?`,
		now, id,
	)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
