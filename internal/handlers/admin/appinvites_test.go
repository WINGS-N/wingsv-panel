package admin

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"v.wingsnet.org/internal/storage"
)

func normalizeUpper(value string) string { return strings.ToUpper(value) }

func zeroTime() time.Time { return time.Time{} }

// inviteTree ставит аккаунт в дерево: пришедшему по коду приглашать можно, и
// донорский порог к нему уже не применяется
func inviteTree(t *testing.T, h *Handler, admin storage.Admin) {
	t.Helper()
	host, err := h.store.CreateAccount("host", "hash", false, storage.RoleAdmin, false)
	if err != nil {
		t.Fatal(err)
	}
	invite, err := h.store.CreateInviteWithUses("HOSTCODE00000001", zeroTime(), host.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.store.RedeemInvite(invite.Token, admin.ID); err != nil {
		t.Fatal(err)
	}
}

// Приложение выписывает код без потолка: считать по нему людей в момент встречи
// некому
func TestAppInviteHasNoUseCap(t *testing.T) {
	h, admin := appHandler(t)
	inviteTree(t, h, admin)
	res := postJSON(t, h.requireAuthFor(admin, h.handleAppInvites), "/api/app/invites", nil, "")
	if res.Code != http.StatusCreated {
		t.Fatalf("создание: %d %s", res.Code, res.Body.String())
	}
	var body struct {
		Token   string `json:"token"`
		MaxUses int64  `json:"max_uses"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.MaxUses != 0 {
		t.Fatalf("max_uses = %d, а код должен быть без потолка", body.MaxUses)
	}
	if len(body.Token) != 16 {
		t.Fatalf("код длиной %d, ждали 16 знаков HEX", len(body.Token))
	}
	if body.Token != normalizeUpper(body.Token) {
		t.Fatalf("код не заглавными: %s", body.Token)
	}
}

// Коды не копятся бесконечно: при выписке нового лишние отзываются
func TestAppInvitesAreCapped(t *testing.T) {
	h, admin := appHandler(t)
	inviteTree(t, h, admin)
	for i := 0; i < appInviteKeep+3; i++ {
		res := postJSON(t, h.requireAuthFor(admin, h.handleAppInvites), "/api/app/invites", nil, "")
		if res.Code != http.StatusCreated {
			t.Fatalf("создание %d: %d %s", i, res.Code, res.Body.String())
		}
	}
	invites, err := h.store.ListInvitesByAdmin(admin.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(invites) > appInviteKeep {
		t.Fatalf("осталось %d кодов, а держим не больше %d", len(invites), appInviteKeep)
	}
}

// Погашенный код никуда не девается: по нему уже прошли люди, и след должен жить
func TestSpentInvitesSurviveTheCleanup(t *testing.T) {
	h, admin := appHandler(t)
	inviteTree(t, h, admin)
	spent, err := h.store.CreateInviteWithUses("SPENTCODE0000001", zeroTime(), admin.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	guest, err := h.store.CreateAccount("guest", "hash", false, storage.RoleAdmin, false)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.store.RedeemInvite(spent.Token, guest.ID); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < appInviteKeep+2; i++ {
		postJSON(t, h.requireAuthFor(admin, h.handleAppInvites), "/api/app/invites", nil, "")
	}
	if _, err := h.store.FindInvite(spent.Token); err != nil {
		t.Fatal("использованный код снесли вместе с лишними")
	}
}
