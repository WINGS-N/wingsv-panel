package configpatch

import (
	"testing"

	wingsvpb "v.wingsnet.org/internal/gen/wingsvpb"
)

// Патч несёт только тронутые поля, и пути считаются по ним, а не со слов клиента
func TestPathsListsOnlyWhatIsSet(t *testing.T) {
	patch := &wingsvpb.Config{
		Ver:  1,
		Type: wingsvpb.ConfigType_CONFIG_TYPE_XRAY,
		Xray: &wingsvpb.Xray{
			Settings: &wingsvpb.XraySettings{RemoteDns: "77.88.8.8"},
		},
	}
	paths := Paths(patch)
	if len(paths) != 1 || paths[0] != "xray.settings.remote_dns" {
		t.Fatalf("пути = %v, ждали один xray.settings.remote_dns", paths)
	}
}

// Соседнее поле правкой не считается: слитый конфиг сохраняет остальное
func TestApplyKeepsUntouchedFields(t *testing.T) {
	stored := &wingsvpb.Config{
		Ver: 1,
		Xray: &wingsvpb.Xray{
			Settings:        &wingsvpb.XraySettings{RemoteDns: "1.1.1.1", DirectDns: "8.8.8.8"},
			ActiveProfileId: "old",
		},
	}
	patch := &wingsvpb.Config{
		Type: wingsvpb.ConfigType_CONFIG_TYPE_XRAY,
		Xray: &wingsvpb.Xray{Settings: &wingsvpb.XraySettings{RemoteDns: "77.88.8.8"}},
	}
	merged, err := Apply(stored, patch)
	if err != nil {
		t.Fatal(err)
	}
	if merged.GetXray().GetSettings().GetRemoteDns() != "77.88.8.8" {
		t.Fatal("правка не применилась")
	}
	if merged.GetXray().GetSettings().GetDirectDns() != "8.8.8.8" {
		t.Fatal("соседнее поле затёрлось")
	}
	if merged.GetXray().GetActiveProfileId() != "old" {
		t.Fatal("активный профиль затёрся")
	}
}

// Список заменяется целиком: половина старого рядом с половиной нового - мусор
func TestApplyReplacesListsInsteadOfAppending(t *testing.T) {
	stored := &wingsvpb.Config{
		Xray: &wingsvpb.Xray{Profiles: []*wingsvpb.VlessProfile{{Id: "a"}, {Id: "b"}}},
	}
	patch := &wingsvpb.Config{
		Type: wingsvpb.ConfigType_CONFIG_TYPE_XRAY,
		Xray: &wingsvpb.Xray{Profiles: []*wingsvpb.VlessProfile{{Id: "c"}}},
	}
	merged, err := Apply(stored, patch)
	if err != nil {
		t.Fatal(err)
	}
	got := merged.GetXray().GetProfiles()
	if len(got) != 1 || got[0].GetId() != "c" {
		t.Fatalf("список = %d элементов, ждали ровно один новый", len(got))
	}
}

// Пустой патч менять нечего
func TestApplyRefusesEmptyPatch(t *testing.T) {
	if _, err := Apply(&wingsvpb.Config{}, &wingsvpb.Config{Type: wingsvpb.ConfigType_CONFIG_TYPE_XRAY}); err == nil {
		t.Fatal("пустой патч прошёл")
	}
}

// Конфликт считается по полям: правка соседнего поля не мешает
func TestConflictsAreCountedPerField(t *testing.T) {
	touched := map[string]int64{
		"xray.settings.remote_dns": 7,
		"xray.settings.direct_dns": 4,
	}
	// Редактор открывали на версии 5: чужая правка remote_dns новее
	if got := Conflicts([]string{"xray.settings.remote_dns"}, touched, 5); len(got) != 1 {
		t.Fatalf("конфликт не найден: %v", got)
	}
	// Соседнее поле трогали раньше - спорить не о чем
	if got := Conflicts([]string{"xray.settings.direct_dns"}, touched, 5); len(got) != 0 {
		t.Fatalf("нашли конфликт на ровном месте: %v", got)
	}
	// Первая правка вообще без базы конфликтовать не может
	if got := Conflicts([]string{"xray.settings.remote_dns"}, touched, 0); len(got) != 0 {
		t.Fatalf("конфликт без базовой версии: %v", got)
	}
}

// Отметки о правке копятся по путям
func TestTouchRecordsVersions(t *testing.T) {
	touched := Touch(nil, []string{"a", "b"}, 3)
	touched = Touch(touched, []string{"b"}, 9)
	if touched["a"] != 3 || touched["b"] != 9 {
		t.Fatalf("версии полей = %v", touched)
	}
}
