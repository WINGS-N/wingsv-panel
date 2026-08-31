package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Шрифты нужны и странице подписки федерации, у которой свой домен
func TestCorsOpensFontsToAnyOrigin(t *testing.T) {
	handler := cors("https://v.wingsnet.org", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	request := httptest.NewRequest(http.MethodGet, "/fonts/samsungsharpsans_bold.otf", nil)
	request.Header.Set("Origin", "https://federation.wingsnet.org")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("fonts allow-origin = %q, want *", got)
	}
	if got := recorder.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Fatalf("fonts must not carry credentials, got %q", got)
	}

	// Чужой origin на обычном пути по-прежнему не получает ничего
	request = httptest.NewRequest(http.MethodGet, "/api/admin/clients", nil)
	request.Header.Set("Origin", "https://federation.wingsnet.org")
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if got := recorder.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("api allow-origin = %q, want empty", got)
	}
}
