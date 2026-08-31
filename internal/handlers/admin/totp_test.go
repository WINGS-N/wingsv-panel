package admin

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"

	"v.wingsnet.org/internal/storage"
)

func enableTOTP(t *testing.T, h *Handler, admin storage.Admin) []string {
	t.Helper()
	start := postJSON(t, h.requireAuthFor(admin, h.handleTOTP), "/api/admin/me/totp", nil, "")
	if start.Code != http.StatusOK {
		t.Fatalf("старт: %d %s", start.Code, start.Body.String())
	}
	var setup struct {
		Secret string `json:"secret"`
	}
	if err := json.Unmarshal(start.Body.Bytes(), &setup); err != nil {
		t.Fatal(err)
	}
	code, err := totp.GenerateCode(setup.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	confirm := putJSON(t, h.requireAuthFor(admin, h.handleTOTP), "/api/admin/me/totp", map[string]string{"code": code})
	if confirm.Code != http.StatusOK {
		t.Fatalf("подтверждение: %d %s", confirm.Code, confirm.Body.String())
	}
	var body struct {
		BackupCodes []string `json:"backup_codes"`
	}
	if err := json.Unmarshal(confirm.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.BackupCodes
}

// Включённый второй фактор не пускает по одному паролю и принимает свежий код
func TestSecondFactorGatesTheLogin(t *testing.T) {
	h, admin := appHandler(t)
	enableTOTP(t, h, admin)

	if h.verifySecondFactor(admin, "") {
		t.Fatal("пустой код прошёл")
	}
	if h.verifySecondFactor(admin, "000000") {
		t.Fatal("случайный код прошёл")
	}
	state, err := h.store.TOTPFor(admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	code, err := totp.GenerateCode(state.Secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !h.verifySecondFactor(admin, code) {
		t.Fatal("правильный код не подошёл")
	}
}

// Резервный код работает ровно один раз
func TestBackupCodeBurnsAfterUse(t *testing.T) {
	h, admin := appHandler(t)
	codes := enableTOTP(t, h, admin)
	if len(codes) == 0 {
		t.Fatal("резервные коды не выданы")
	}
	if !h.verifySecondFactor(admin, codes[0]) {
		t.Fatal("резервный код не сработал")
	}
	if h.verifySecondFactor(admin, codes[0]) {
		t.Fatal("резервный код сработал дважды")
	}
	if left := h.store.BackupCodesLeft(admin.ID); left != len(codes)-1 {
		t.Fatalf("осталось %d из %d", left, len(codes))
	}
}

// Аккаунт без второго фактора проходит без кода
func TestAccountWithoutSecondFactorPassesThrough(t *testing.T) {
	h, admin := appHandler(t)
	if !h.verifySecondFactor(admin, "") {
		t.Fatal("аккаунт без второго фактора требует код")
	}
}

// Вход в приложение просит код, когда фактор включён
func TestAppLoginAsksForTheCode(t *testing.T) {
	h, admin := appHandler(t)
	enableTOTP(t, h, admin)

	rec := postJSON(t, h.handleAppLogin, "/api/app/login",
		map[string]string{"username": admin.Username, "password": "s3cret-pass"}, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("статус = %d", rec.Code)
	}
	var body struct {
		TOTPRequired bool `json:"totp_required"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if !body.TOTPRequired {
		t.Fatalf("панель не сказала, что нужен код: %s", rec.Body.String())
	}

	state, _ := h.store.TOTPFor(admin.ID)
	code, _ := totp.GenerateCode(state.Secret, time.Now())
	ok := postJSON(t, h.handleAppLogin, "/api/app/login",
		map[string]string{"username": admin.Username, "password": "s3cret-pass", "code": code}, "")
	if ok.Code != http.StatusOK {
		t.Fatalf("вход с кодом не прошёл: %d %s", ok.Code, ok.Body.String())
	}
}
