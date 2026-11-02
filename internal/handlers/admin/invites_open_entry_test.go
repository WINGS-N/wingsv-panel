package admin

import (
	"context"
	"net/http"
	"testing"

	"v.wingsnet.org/internal/storage"
)

// Открытый вход снимает ровно одно требование - быть приглашённым. Право
// выписывать коды он не раздаёт: донорский порог там свой и остаётся на месте.
func TestOpenEntryDoesNotHandOutTheInviteRight(t *testing.T) {
	h, admin := appHandler(t)
	if err := h.store.SetPlatformSetting(storage.SettingFederationOpenEntry, "true"); err != nil {
		t.Fatal(err)
	}

	res := postJSON(t, h.requireAuthFor(admin, h.handleInvites), "/api/invites", nil, "")
	if res.Code != http.StatusForbidden {
		t.Fatalf("открытый вход выдал право приглашать: %d %s", res.Code, res.Body.String())
	}
}

// Пришедший по приглашению выписывает коды независимо от настройки: он уже
// ветвь дерева, и за него отвечает пригласивший.
func TestInvitedAdminStillInvitesWithOpenEntryOff(t *testing.T) {
	h, admin := appHandler(t)
	inviteTree(t, h, admin)

	res := postJSON(t, h.requireAuthFor(admin, h.handleInvites), "/api/invites", nil, "")
	if res.Code != http.StatusCreated {
		t.Fatalf("приглашённый не смог выписать код: %d %s", res.Code, res.Body.String())
	}
}

// Без приглашения федерация закрыта: аккаунт, которого никто не звал и который
// ничего не отдал, участником не становится.
func TestFederationNeedsAnInviteByDefault(t *testing.T) {
	h, admin := appHandler(t)

	if err := h.mayUseFederation(context.Background(), admin); err == nil {
		t.Fatal("без приглашения и без настройки федерация пустила")
	}
}

// Ради этого всё и затевалось: владелец включил открытый вход, и требование
// приглашения снято. Дерево при этом никуда не делось, коды работают как прежде.
func TestOpenEntryDropsTheInviteRequirement(t *testing.T) {
	h, admin := appHandler(t)
	if err := h.store.SetPlatformSetting(storage.SettingFederationOpenEntry, "true"); err != nil {
		t.Fatal(err)
	}

	if err := h.mayUseFederation(context.Background(), admin); err != nil {
		t.Fatalf("с открытым входом федерация всё ещё требует приглашение: %v", err)
	}

	if err := h.store.SetPlatformSetting(storage.SettingFederationOpenEntry, "false"); err != nil {
		t.Fatal(err)
	}
	if err := h.mayUseFederation(context.Background(), admin); err == nil {
		t.Fatal("выключили настройку, а требование не вернулось")
	}
}
