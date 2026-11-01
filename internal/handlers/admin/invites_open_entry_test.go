package admin

import (
	"net/http"
	"testing"

	"v.wingsnet.org/internal/storage"
)

// Без приглашения и без настройки человек встаёт в дерево корнем, и с него
// требуют донорский порог: федерация выключена, так что приглашать нечем.
func TestRootEntryIsClosedByDefault(t *testing.T) {
	h, admin := appHandler(t)

	res := postJSON(t, h.requireAuthFor(admin, h.handleInvites), "/api/invites", nil, "")
	if res.Code != http.StatusForbidden {
		t.Fatalf("без приглашения и без настройки код создался: %d %s", res.Code, res.Body.String())
	}
}

// Владелец включил открытый вход - донорский порог с корня снят, и код выписывается.
// Сами приглашения при этом никуда не делись, ими по-прежнему встают в дерево ветвью.
func TestOpenEntryLetsAnyoneInviteWithoutDonating(t *testing.T) {
	h, admin := appHandler(t)
	if err := h.store.SetPlatformSetting(storage.SettingFederationOpenEntry, "true"); err != nil {
		t.Fatal(err)
	}

	res := postJSON(t, h.requireAuthFor(admin, h.handleInvites), "/api/invites", nil, "")
	if res.Code != http.StatusCreated {
		t.Fatalf("с открытым входом код не создался: %d %s", res.Code, res.Body.String())
	}

	if err := h.store.SetPlatformSetting(storage.SettingFederationOpenEntry, "false"); err != nil {
		t.Fatal(err)
	}
	if res := postJSON(t, h.requireAuthFor(admin, h.handleInvites), "/api/invites", nil, ""); res.Code != http.StatusForbidden {
		t.Fatalf("выключили настройку, а порог не вернулся: %d", res.Code)
	}
}

// Пришедший по приглашению приглашает независимо от настройки: он уже ветвь дерева,
// и за него отвечает пригласивший.
func TestInvitedAdminStillInvitesWithOpenEntryOff(t *testing.T) {
	h, admin := appHandler(t)
	inviteTree(t, h, admin)

	res := postJSON(t, h.requireAuthFor(admin, h.handleInvites), "/api/invites", nil, "")
	if res.Code != http.StatusCreated {
		t.Fatalf("приглашённый не смог выписать код: %d %s", res.Code, res.Body.String())
	}
}
