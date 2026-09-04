package owner

import (
	"context"
	"encoding/json"
	"net/http"

	"v.wingsnet.org/internal/gen/headpb"
	"v.wingsnet.org/internal/storage"
)

// Разметку смотрит владелец: на ней учится бустинг, который потом режет людям
// доступ. Машина на пограничных случаях плавает, и непроверенная метка означает,
// что модель выучит её ошибку и станет повторять уверенно

type labelView struct {
	ID        uint64             `json:"id"`
	AtUnix    int64              `json:"at_unix"`
	SubjectID string             `json:"subject_id"`
	Label     int32              `json:"label"`
	LabelBy   string             `json:"label_by"`
	Why       string             `json:"why"`
	Values    map[string]float64 `json:"values"`
}

type labelsResponse struct {
	Enabled bool        `json:"enabled"`
	Labels  []labelView `json:"labels"`
	Total   uint32      `json:"total"`
	// ByMachine и ByHuman показывают, сколько уже проверено: по этим двум
	// числам видно, можно ли на разметке учить
	ByMachine uint32 `json:"by_machine"`
	ByHuman   uint32 `json:"by_human"`
	Error     string `json:"error,omitempty"`
}

// handleOracleLabels отдаёт разметку и принимает приговор человека
func (h *Handler) handleOracleLabels(w http.ResponseWriter, r *http.Request, _ storage.Admin) {
	if h.fed == nil || !h.fed.Enabled() {
		writeJSON(w, http.StatusOK, labelsResponse{})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), headTimeout)
	defer cancel()

	switch r.Method {
	case http.MethodGet:
		limit, offset := pageParams(r, 25)
		got, err := h.fed.OracleLabels(ctx, limit, offset, r.URL.Query().Get("accused") == "1")
		if err != nil {
			// Башка без хранилища снимков отвечает Unimplemented, и это
			// состояние, а не поломка
			writeJSON(w, http.StatusOK, labelsResponse{Error: "разметка пока не поддерживается: " + err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, labelsOf(got))
	case http.MethodPost:
		var req struct {
			ID    uint64 `json:"id"`
			Label int32  `json:"label"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "не разобрал запрос")
			return
		}
		got, err := h.fed.SetOracleLabel(ctx, req.ID, req.Label)
		if err != nil {
			writeError(w, http.StatusBadGateway, "голова федерации недоступна: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, labelsOf(got))
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func labelsOf(got *headpb.OracleLabelsResponse) labelsResponse {
	out := labelsResponse{
		Enabled:   true,
		Total:     got.GetTotal(),
		ByMachine: got.GetByMachine(),
		ByHuman:   got.GetByHuman(),
	}
	for _, l := range got.GetLabels() {
		out.Labels = append(out.Labels, labelView{
			ID: l.GetId(), AtUnix: l.GetAtUnix(), SubjectID: l.GetSubjectId(),
			Label: l.GetLabel(), LabelBy: l.GetLabelBy(), Why: l.GetWhy(),
			Values: l.GetValues(),
		})
	}
	return out
}
