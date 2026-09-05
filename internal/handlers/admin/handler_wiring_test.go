package admin

import (
	"testing"

	"v.wingsnet.org/internal/config"
)

// Поле, забытое в конструкторе, роняет прод при первом же запросе: пустой стол
// заявок это nil, а не пустая карта. Тест дешевле разбора паники в бою
func TestEveryDeskIsBuilt(t *testing.T) {
	h := New(config.Config{PublicBaseURL: "https://panel.example"}, nil, nil, nil)
	if h.qr == nil {
		t.Error("стол заявок по QR не заведён")
	}
	if h.halfway == nil {
		t.Error("стол недошедших входов не заведён")
	}
	if h.appCodes == nil {
		t.Error("коды приложения не заведены")
	}
	if h.session == nil {
		t.Error("клиент сервиса учёток не заведён")
	}
	if h.oidc == nil {
		t.Error("клиент входа не заведён")
	}
}
