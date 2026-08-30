package admin

import "testing"

// Флот исполняет то, что тут разрешат. Апстримовый Xray не несёт патчей WINGS -
// нода с ним выглядит здоровой, а статистика по пирам, на которой стоит весь
// учёт бюджета, остаётся пустой.
func TestOnlyOurForkIsAccepted(t *testing.T) {
	ours := "https://github.com/WINGS-N/Xray-core/releases/download/v26.7.11-wv/Xray-linux-64.zip"
	if err := fromOurFork(ours, xrayForkRepo); err != nil {
		t.Fatalf("наш собственный релиз отвергнут: %v", err)
	}

	for _, bad := range []string{
		"https://github.com/XTLS/Xray-core/releases/download/v1.8.0/Xray-linux-64.zip",
		"https://evil.example/WINGS-N/Xray-core/releases/download/v1/Xray-linux-64.zip",
		"https://github.com/WINGS-N/Xray-core-evil/releases/download/v1/Xray-linux-64.zip",
	} {
		if err := fromOurFork(bad, xrayForkRepo); err == nil {
			t.Errorf("принят чужой билд: %s", bad)
		}
	}
}

// Пустой url означает "не трогать сборку", а не "поставить ничего": иначе
// сохранение любой другой настройки снесло бы флоту Xray.
func TestAnEmptyURLIsLeftAlone(t *testing.T) {
	if err := fromOurFork("", xrayForkRepo); err != nil {
		t.Errorf("пустой url отвергнут: %v", err)
	}
}
