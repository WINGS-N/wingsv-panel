package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"v.wingsnet.org/internal/storage"
)

func getDonations(t *testing.T, h *Handler, admin storage.Admin) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/admin/donations", nil)
	rec := httptest.NewRecorder()
	h.requireAuthFor(admin, h.handleDonations)(rec, req)
	return rec
}

// Выключенная денежная часть оставляет один адрес на разработку: котла, кода
// для memo и истории заносов в ответе больше нет.
func TestDonationsKeepOnlyTheDevWalletWhenCryptoIsOff(t *testing.T) {
	h, admin := appHandler(t)
	if err := h.store.SetPlatformSetting(settingTrafficWallet, "TraffiCWalleTAddress1111111111111111111111"); err != nil {
		t.Fatal(err)
	}
	if err := h.store.SetPlatformSetting(storage.SettingCryptoEnabled, "false"); err != nil {
		t.Fatal(err)
	}

	res := getDonations(t, h, admin)
	if res.Code != http.StatusOK {
		t.Fatalf("код = %d %s", res.Code, res.Body.String())
	}
	var body donationsResponse
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Wallets.Dev != defaultDevWallet {
		t.Fatalf("адрес на разработку = %q, он должен оставаться", body.Wallets.Dev)
	}
	if body.Wallets.Traffic != "" {
		t.Fatalf("котёл остался виден: %q", body.Wallets.Traffic)
	}
	if body.Memo != "" || body.CutBps != 0 || len(body.Mine) != 0 {
		t.Fatalf("осталась механика: memo=%q cut=%d заносов=%d", body.Memo, body.CutBps, len(body.Mine))
	}
	if body.CryptoEnabled {
		t.Fatal("ответ говорит, что денежная часть включена")
	}
}

// Пока тумблер не трогали, денежная часть считается включённой.
func TestCryptoStaysOnByDefault(t *testing.T) {
	h, _ := appHandler(t)
	if !h.cryptoOn() {
		t.Fatal("установка без тумблера потеряла денежную часть")
	}
	if wallets := h.donationWallets(); wallets.Dev != defaultDevWallet {
		t.Fatalf("адрес на разработку = %q", wallets.Dev)
	}
	if err := h.store.SetPlatformSetting(storage.SettingCryptoEnabled, "false"); err != nil {
		t.Fatal(err)
	}
	if h.cryptoOn() {
		t.Fatal("тумблер выключен, а денежная часть жива")
	}
}

// Ручки, которые двигают деньги, при выключенной крипте закрыты.
func TestCryptoRoutesAreClosedWhenOff(t *testing.T) {
	h, admin := appHandler(t)
	if err := h.store.SetPlatformSetting(storage.SettingCryptoEnabled, "false"); err != nil {
		t.Fatal(err)
	}

	guarded := h.requireAuthFor(admin, h.requireCrypto(func(w http.ResponseWriter, _ *http.Request, _ storage.Admin) {
		w.WriteHeader(http.StatusOK)
	}))
	res := postJSON(t, guarded, "/api/admin/donations/claim", map[string]string{"signature": "x"}, "")
	if res.Code != http.StatusForbidden {
		t.Fatalf("код = %d, ждали отказ", res.Code)
	}
}
