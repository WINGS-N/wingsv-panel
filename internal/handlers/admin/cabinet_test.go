package admin

import (
	"encoding/json"
	"net/http"
	"testing"

	"v.wingsnet.org/internal/storage"
)

func memberOf(t *testing.T, h *Handler, id int64) storage.Admin {
	t.Helper()
	if err := h.store.SetPanelAccess(id, false); err != nil {
		t.Fatal(err)
	}
	member, err := h.store.FindAdminByID(id)
	if err != nil {
		t.Fatal(err)
	}
	return member
}

// Разрешения на админку по умолчанию не спрашивают: кто уже в дереве, тот
// вправе вести своих клиентов
func TestPanelOpensItselfByDefault(t *testing.T) {
	h, admin := appHandler(t)
	member := memberOf(t, h, admin.ID)

	res := postJSON(t, h.requireAuthFor(member, h.handlePanelAccess), "/api/admin/me/panel-access", nil, "")
	if res.Code != http.StatusOK {
		t.Fatalf("код = %d %s", res.Code, res.Body.String())
	}
	var body struct {
		Granted   bool `json:"granted"`
		Requested bool `json:"requested"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Granted || body.Requested {
		t.Fatalf("granted=%v requested=%v, ждали выданный доступ", body.Granted, body.Requested)
	}
	after, err := h.store.FindAdminByID(member.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !after.PanelAccess {
		t.Fatal("панель не открылась")
	}
}

// С включённой модерацией просьба уходит владельцу, а панель остаётся закрытой
func TestPanelGoesThroughTheOwnerWhenModerated(t *testing.T) {
	h, admin := appHandler(t)
	member := memberOf(t, h, admin.ID)
	if err := h.store.SetPlatformSetting(storage.SettingPanelByRequest, "true"); err != nil {
		t.Fatal(err)
	}

	res := postJSON(t, h.requireAuthFor(member, h.handlePanelAccess), "/api/admin/me/panel-access", nil, "")
	if res.Code != http.StatusOK {
		t.Fatalf("код = %d %s", res.Code, res.Body.String())
	}
	var body struct {
		Granted   bool `json:"granted"`
		Requested bool `json:"requested"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Granted || !body.Requested {
		t.Fatalf("granted=%v requested=%v, ждали заявку", body.Granted, body.Requested)
	}
	after, err := h.store.FindAdminByID(member.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.PanelAccess {
		t.Fatal("модерация не удержала панель закрытой")
	}
	if after.PanelRequestedAt.IsZero() {
		t.Fatal("заявка не записалась")
	}

	// Повторная просьба не двигает отметку: очередь идёт по времени первой
	postJSON(t, h.requireAuthFor(member, h.handlePanelAccess), "/api/admin/me/panel-access", nil, "")
	again, err := h.store.FindAdminByID(member.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !again.PanelRequestedAt.Equal(after.PanelRequestedAt) {
		t.Fatal("повторная просьба сдвинула отметку")
	}
}

// У кого панель и так есть, просить нечего
func TestPanelAccessRejectedWhenAlreadyGranted(t *testing.T) {
	h, admin := appHandler(t)
	res := postJSON(t, h.requireAuthFor(admin, h.handlePanelAccess), "/api/admin/me/panel-access", nil, "")
	if res.Code != http.StatusBadRequest {
		t.Fatalf("код = %d, want 400", res.Code)
	}
}
