package admin

import (
	"encoding/json"
	"net/http"

	"v.wingsnet.org/internal/storage"
)

// handleVKLinks serves the admin's shared VK Links pool: GET lists them, PUT
// replaces the list. The links are embedded (merge-only) into every managed
// wingsv:// link the admin issues, so the app adds them to its own settings.
func (h *Handler) handleVKLinks(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	switch r.Method {
	case http.MethodGet:
		links, err := h.store.GetAdminVKLinks(admin.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"links": links})
	case http.MethodPut:
		var req struct {
			Links []string `json:"links"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid body")
			return
		}
		if err := h.store.SetAdminVKLinks(admin.ID, req.Links); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		links, err := h.store.GetAdminVKLinks(admin.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"links": links})
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}
