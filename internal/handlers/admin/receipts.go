package admin

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"

	fedpb "v.wingsnet.org/internal/gen/fedpb"
	"v.wingsnet.org/internal/storage"
)

// maxReceiptsPerCall ограничивает пачку. Телефон копит расписки офлайн, но
// десяток окон это уже сутки молчания, а не задержка сети
const maxReceiptsPerCall = 48

// handleAppReceiptKey принимает публичную половину ключа устройства.
//
// Приватная не уезжает никуда и никогда: уедет - и расписку сможет высрать кто
// угодно, а вся затея с ними ровно в том, что нода этого не может
func (h *Handler) handleAppReceiptKey(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.fed == nil || !h.fed.Enabled() {
		writeError(w, http.StatusNotFound, "федерация выключена")
		return
	}
	var req struct {
		PublicKey string `json:"public_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad request body")
		return
	}
	key, err := base64.StdEncoding.DecodeString(req.PublicKey)
	if err != nil || len(key) != 32 {
		writeError(w, http.StatusBadRequest, "ключ должен быть 32 байтами ed25519 в base64")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), federationTimeout)
	defer cancel()
	changed, err := h.fed.RegisterKey(ctx, federationUserID(admin), key)
	if err != nil {
		writeError(w, http.StatusBadGateway, "голова федерации недоступна: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "changed": changed})
}

// handleAppReceipts принимает подписанные расписки о трафике
func (h *Handler) handleAppReceipts(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.fed == nil || !h.fed.Enabled() {
		writeError(w, http.StatusNotFound, "федерация выключена")
		return
	}
	var req struct {
		// ClientIP - адрес, который устройство замерило у себя мимо туннеля.
		// Сверяется с тем, что видят ноды
		ClientIP string `json:"client_ip"`
		Receipts []struct {
			ClientID        string `json:"client_id"`
			NodeID          string `json:"node_id"`
			Transport       string `json:"transport"`
			WindowStartUnix int64  `json:"window_start_unix"`
			WindowEndUnix   int64  `json:"window_end_unix"`
			PayloadUpBytes  uint64 `json:"payload_up_bytes"`
			PayloadDown     uint64 `json:"payload_down_bytes"`
			Nonce           string `json:"nonce"`
			Signature       string `json:"signature"`
		} `json:"receipts"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad request body")
		return
	}
	if len(req.Receipts) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{"accepted": 0, "rejected": 0})
		return
	}
	if len(req.Receipts) > maxReceiptsPerCall {
		req.Receipts = req.Receipts[:maxReceiptsPerCall]
	}

	out := make([]*fedpb.TrafficReceipt, 0, len(req.Receipts))
	for _, item := range req.Receipts {
		signature, err := base64.StdEncoding.DecodeString(item.Signature)
		if err != nil {
			writeError(w, http.StatusBadRequest, "подпись не в base64")
			return
		}
		out = append(out, &fedpb.TrafficReceipt{
			ClientId: item.ClientID, NodeId: item.NodeID, Transport: item.Transport,
			WindowStartUnix: item.WindowStartUnix, WindowEndUnix: item.WindowEndUnix,
			PayloadUpBytes: item.PayloadUpBytes, PayloadDownBytes: item.PayloadDown,
			Nonce: item.Nonce, Signature: signature,
		})
	}

	ctx, cancel := context.WithTimeout(r.Context(), federationTimeout)
	defer cancel()
	got, err := h.fed.SubmitReceipts(ctx, federationUserID(admin), out, req.ClientIP)
	if err != nil {
		writeError(w, http.StatusBadGateway, "голова федерации недоступна: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"accepted": got.GetAccepted(),
		"rejected": got.GetRejected(),
		"reason":   got.GetReason(),
	})
}
