package owner

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"v.wingsnet.org/internal/gen/headpb"
	"v.wingsnet.org/internal/storage"
)

// Купленные подписки видит и правит только владелец площадки: это его деньги,
// его аккаунт у продавца, и бан за чужие художества прилетит туда же

type upstreamView struct {
	ID     string `json:"id"`
	Vendor string `json:"vendor"`
	URL    string `json:"url"`
	// DeviceID - чем башка представляется продавцу. Всегда одно и то же, иначе
	// на той стороне это выглядит как толпа новых железок
	DeviceID    string `json:"device_id"`
	MaxClients  uint32 `json:"max_clients"`
	Enabled     bool   `json:"enabled"`
	Links       uint32 `json:"links"`
	FetchedUnix int64  `json:"fetched_unix"`
	LastError   string `json:"last_error"`
}

type upstreamsResponse struct {
	Enabled bool           `json:"enabled"`
	Sources []upstreamView `json:"sources"`
	// MinConfidence - какое доверие нужно человеку, чтобы его вообще пустили на
	// купленный сервер
	MinConfidence uint32 `json:"min_confidence"`
	Error         string `json:"error,omitempty"`
}

// handleUpstreams отдаёт и правит купленные подписки
func (h *Handler) handleUpstreams(w http.ResponseWriter, r *http.Request, _ storage.Admin) {
	if h.fed == nil || !h.fed.Enabled() {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), headTimeout)
	defer cancel()

	var (
		got *headpb.UpstreamsResponse
		err error
	)
	switch r.Method {
	case http.MethodGet:
		got, err = h.fed.Upstreams(ctx)
	case http.MethodPost:
		var req struct {
			Source    *upstreamView `json:"source"`
			EnableAll *bool         `json:"enable_all"`
			RemoveID  string        `json:"remove_id"`
		}
		if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr != nil {
			writeError(w, http.StatusBadRequest, "не разобрал запрос")
			return
		}
		switch {
		case req.EnableAll != nil:
			got, err = h.fed.EnableUpstreams(ctx, *req.EnableAll)
		case req.RemoveID != "":
			got, err = h.fed.RemoveUpstream(ctx, req.RemoveID)
		case req.Source != nil:
			got, err = h.fed.PutUpstream(ctx, &headpb.UpstreamSource{
				Id:         strings.TrimSpace(req.Source.ID),
				Vendor:     strings.TrimSpace(req.Source.Vendor),
				Url:        strings.TrimSpace(req.Source.URL),
				DeviceId:   strings.TrimSpace(req.Source.DeviceID),
				MaxClients: req.Source.MaxClients,
				Enabled:    req.Source.Enabled,
			})
		default:
			writeError(w, http.StatusBadRequest, "нечего делать")
			return
		}
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if err != nil {
		// Башка без этой возможности отвечает Unimplemented, и это состояние, а
		// не поломка: раздел остаётся с объяснением вместо списка
		writeJSON(w, http.StatusOK, upstreamsResponse{
			Error: "купленные подписки пока не поддерживаются: " + err.Error(),
		})
		return
	}

	out := upstreamsResponse{Enabled: got.GetEnabled(), MinConfidence: got.GetMinConfidence()}
	for _, source := range got.GetSources() {
		out.Sources = append(out.Sources, upstreamView{
			ID: source.GetId(), Vendor: source.GetVendor(), URL: source.GetUrl(),
			DeviceID: source.GetDeviceId(), MaxClients: source.GetMaxClients(),
			Enabled: source.GetEnabled(), Links: source.GetLinks(),
			FetchedUnix: source.GetFetchedUnix(), LastError: source.GetLastError(),
		})
	}
	writeJSON(w, http.StatusOK, out)
}
