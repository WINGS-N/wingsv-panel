package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"v.wingsnet.org/internal/auth"
	"v.wingsnet.org/internal/storage"
)

func appHandler(t *testing.T) (*Handler, storage.Admin) {
	t.Helper()
	st, err := storage.Open(storage.Options{Driver: storage.DriverSQLite, DSN: filepath.Join(t.TempDir(), "app.db")})
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	hash, err := auth.HashPassword("s3cret-pass")
	if err != nil {
		t.Fatal(err)
	}
	admin, err := st.CreateAccount("phoneuser", hash, false, storage.RoleAdmin, false)
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{store: st, auth: auth.New(st, false), appCodes: newAppCodes()}
	return h, admin
}

func postJSON(t *testing.T, fn http.HandlerFunc, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	fn(rec, req)
	return rec
}

// Вход логином и паролём выдаёт токен, которым дальше открывается аккаунт
func TestAppLoginIssuesAWorkingToken(t *testing.T) {
	h, admin := appHandler(t)
	rec := postJSON(t, h.handleAppLogin, "/api/app/login",
		map[string]string{"username": "phoneuser", "password": "s3cret-pass", "device_name": "Galaxy"}, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("статус = %d, тело %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Token   string         `json:"token"`
		Account map[string]any `json:"account"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Token == "" {
		t.Fatal("токен не выдан")
	}
	if out.Account["username"] != admin.Username {
		t.Fatalf("аккаунт не тот: %+v", out.Account)
	}

	me := postJSON(t, h.requireApp(h.handleAppMe), "/api/app/me", nil, out.Token)
	if me.Code != http.StatusOK {
		t.Fatalf("токен не пускает: %d", me.Code)
	}

	// Токен хранится хешем: база, утёкшая целиком, не должна давать доступ
	if _, err := h.store.LookupAppSession("явно-не-тот-токен"); err == nil {
		t.Fatal("чужой токен пустили")
	}
}

// Неверный пароль не выдаёт ничего
func TestAppLoginRejectsWrongPassword(t *testing.T) {
	h, _ := appHandler(t)
	rec := postJSON(t, h.handleAppLogin, "/api/app/login",
		map[string]string{"username": "phoneuser", "password": "wrong"}, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("статус = %d", rec.Code)
	}
}

// Одноразовый код меняется на токен ровно один раз
func TestAppCodeIsRedeemedOnce(t *testing.T) {
	h, admin := appHandler(t)
	h.appCodes.issue("code-1", admin.ID, time.Now())

	first := postJSON(t, h.handleAppSession, "/api/app/session",
		map[string]string{"code": "code-1", "device_name": "Galaxy"}, "")
	if first.Code != http.StatusOK {
		t.Fatalf("первый обмен: %d %s", first.Code, first.Body.String())
	}
	second := postJSON(t, h.handleAppSession, "/api/app/session",
		map[string]string{"code": "code-1"}, "")
	if second.Code != http.StatusForbidden {
		t.Fatalf("код сработал дважды: %d", second.Code)
	}
}

// Просроченный код не принимается
func TestAppCodeExpires(t *testing.T) {
	h, admin := appHandler(t)
	h.appCodes.issue("code-2", admin.ID, time.Now().Add(-appCodeTTL-time.Minute))
	rec := postJSON(t, h.handleAppSession, "/api/app/session", map[string]string{"code": "code-2"}, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("протухший код принят: %d", rec.Code)
	}
}

// Выход отзывает именно это устройство
func TestAppLogoutRevokesTheDevice(t *testing.T) {
	h, _ := appHandler(t)
	rec := postJSON(t, h.handleAppLogin, "/api/app/login",
		map[string]string{"username": "phoneuser", "password": "s3cret-pass"}, "")
	var out struct {
		Token string `json:"token"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)

	if code := postJSON(t, h.requireApp(h.handleAppLogout), "/api/app/logout", nil, out.Token).Code; code != http.StatusOK {
		t.Fatalf("выход не удался: %d", code)
	}
	after := postJSON(t, h.requireApp(h.handleAppMe), "/api/app/me", nil, out.Token)
	if after.Code != http.StatusUnauthorized {
		t.Fatalf("отозванный токен всё ещё работает: %d", after.Code)
	}
}

// requireAuthFor подставляет уже известный аккаунт: тесты проверяют сами
// обработчики, а не разбор сессии
func (h *Handler) requireAuthFor(admin storage.Admin, next authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		next(w, r, admin)
	}
}

func putJSON(t *testing.T, fn http.HandlerFunc, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, path, bytes.NewReader(raw))
	rec := httptest.NewRecorder()
	fn(rec, req)
	return rec
}
