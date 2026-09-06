package admin

// Кто просит доступ. Имя и картинку берём из своего списка по ключу, а не из
// адресной строки: подставить туда "Сбербанк" может кто угодно, и человек
// разрешит доступ, глядя на чужое имя

import (
	"net/http"
	"strings"
)

// appClient - одно приложение, которому мы вообще готовы что-то отдать
type appClient struct {
	Name string `json:"name"`
	Icon string `json:"icon"`
}

// knownAppClients - список целиком. Незнакомый ключ показывается безымянным, и
// это правильно: имя, которому нечем подтвердиться, хуже отсутствия имени
var knownAppClients = map[string]appClient{
	"wingsv": {Name: "WINGS V", Icon: "/img/wingsv-icon.webp"},
	"matrix": {Name: "Matrix", Icon: "/img/matrix.svg"},
}

// handleAppClient рассказывает экрану согласия, кто именно постучался
func (h *Handler) handleAppClient(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	key := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("app")))
	client, ok := knownAppClients[key]
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"known": false, "name": "Приложение"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"known": true, "name": client.Name, "icon": client.Icon})
}
