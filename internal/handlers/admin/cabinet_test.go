package admin

import (
	"encoding/json"
	"net/http"
	"testing"
)

// Участник просит панель сам, а решает владелец. Повторная просьба не двигает
// отметку: очередь заявок идёт по времени первой
func TestPanelRequestIsRecordedOnce(t *testing.T) {
	h, admin := appHandler(t)
	if err := h.store.SetPanelAccess(admin.ID, false); err != nil {
		t.Fatal(err)
	}
	member, err := h.store.FindAdminByID(admin.ID)
	if err != nil {
		t.Fatal(err)
	}

	res := postJSON(t, h.requireAuthFor(member, h.handlePanelRequest), "/api/admin/me/panel-request", nil, "")
	if res.Code != http.StatusOK {
		t.Fatalf("заявка: %d %s", res.Code, res.Body.String())
	}
	first, err := h.store.FindAdminByID(member.ID)
	if err != nil {
		t.Fatal(err)
	}
	if first.PanelRequestedAt.IsZero() {
		t.Fatal("заявка не записалась")
	}

	postJSON(t, h.requireAuthFor(member, h.handlePanelRequest), "/api/admin/me/panel-request", nil, "")
	again, err := h.store.FindAdminByID(member.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !again.PanelRequestedAt.Equal(first.PanelRequestedAt) {
		t.Fatal("повторная просьба сдвинула отметку")
	}

	// Выданный доступ снимает заявку
	if err := h.store.SetPanelAccess(member.ID, true); err != nil {
		t.Fatal(err)
	}
	if err := h.store.ClearPanelRequest(member.ID); err != nil {
		t.Fatal(err)
	}
	cleared, err := h.store.FindAdminByID(member.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !cleared.PanelRequestedAt.IsZero() {
		t.Fatal("заявка пережила решение")
	}
}

// У кого панель и так есть, просить нечего
func TestPanelRequestRejectedWhenAlreadyGranted(t *testing.T) {
	h, admin := appHandler(t)
	res := postJSON(t, h.requireAuthFor(admin, h.handlePanelRequest), "/api/admin/me/panel-request", nil, "")
	if res.Code != http.StatusBadRequest {
		t.Fatalf("код = %d, want 400", res.Code)
	}
	var body struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(res.Body.Bytes(), &body)
	if body.Message == "" {
		t.Fatal("отказ без объяснения")
	}
}
