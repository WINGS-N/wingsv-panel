package admin

import (
	"testing"

	wingsvpb "v.wingsnet.org/internal/gen/wingsvpb"
)

// Настройки, выставленные на управляемом профиле, переживают его пересборку:
// панель пересобирает профиль на каждый пуш, и раньше всё, кроме режима сессии,
// уезжало обратно к умолчаниям приложения.
func TestManagedTurnProfileKeepsItsSettings(t *testing.T) {
	turn := applyManagedTurnProfile(nil, "c1", "Dev", []byte("tok"), "relay.example:56000")
	if len(turn.Profiles) != 1 {
		t.Fatalf("профилей %d, нужен один управляемый", len(turn.Profiles))
	}
	inner := turn.Profiles[0].Config
	if inner == nil || inner.SessionMode != wingsvpb.TurnSessionMode_TURN_SESSION_MODE_MUX {
		t.Fatalf("режим сессии у свежего профиля: %v", inner)
	}

	// админ правит профиль: обфускация и отпечаток браузера
	inner.WrapMode = wingsvpb.WrapMode_WRAP_MODE_REQUIRED
	inner.BrowserFingerprint = "chrome"

	rebuilt := applyManagedTurnProfile(turn, "c1", "Dev", []byte("tok"), "relay.example:56000")
	if len(rebuilt.Profiles) != 1 {
		t.Fatalf("после пересборки профилей %d, нужен один", len(rebuilt.Profiles))
	}
	kept := rebuilt.Profiles[0].Config
	if kept == nil {
		t.Fatal("пересборка выбросила настройки профиля")
	}
	if kept.WrapMode != wingsvpb.WrapMode_WRAP_MODE_REQUIRED {
		t.Fatalf("режим WRAP стал %v, был required", kept.WrapMode)
	}
	if kept.BrowserFingerprint != "chrome" {
		t.Fatalf("отпечаток стал %q, был chrome", kept.BrowserFingerprint)
	}
	if kept.SessionMode != wingsvpb.TurnSessionMode_TURN_SESSION_MODE_MUX {
		t.Fatalf("режим сессии стал %v", kept.SessionMode)
	}
}

// Профили, заведённые вручную, пересборка не трогает, и своего конфига у них не
// одалживает.
func TestManagedTurnProfileLeavesManualOnesAlone(t *testing.T) {
	manual := &wingsvpb.TurnProfile{
		Id:     "manual",
		Title:  "Свой",
		Config: &wingsvpb.Turn{BrowserFingerprint: "firefox"},
	}
	turn := &wingsvpb.Turn{Profiles: []*wingsvpb.TurnProfile{manual}}

	rebuilt := applyManagedTurnProfile(turn, "c1", "Dev", []byte("tok"), "relay.example:56000")

	if len(rebuilt.Profiles) != 2 {
		t.Fatalf("профилей %d, нужны ручной и управляемый", len(rebuilt.Profiles))
	}
	for _, p := range rebuilt.Profiles {
		if p.Id == "manual" && p.Config.BrowserFingerprint != "firefox" {
			t.Fatalf("ручной профиль изменился: %v", p.Config)
		}
		if p.Id == "c1" && p.Config.BrowserFingerprint != "" {
			t.Fatalf("управляемый профиль подобрал чужие настройки: %v", p.Config)
		}
	}
}
