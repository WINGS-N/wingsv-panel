package owner

import (
	"context"
	"net/http"

	"v.wingsnet.org/internal/storage"
)

// handleOracleNodes отдаёт доверие к нодам.
//
// Это репутация под линейку выплат, а не под выдачу: из ротации тёмную ноду
// убирают зонды, а тут решается, сколько ей причитается за отданный трафик
func (h *Handler) handleOracleNodes(w http.ResponseWriter, r *http.Request, _ storage.Admin) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if h.fed == nil || !h.fed.Enabled() {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), headTimeout)
	defer cancel()
	limit, offset := pageParams(r, 25)
	got, err := h.fed.OracleNodes(ctx, limit, offset)
	if err != nil {
		writeError(w, http.StatusBadGateway, "голова федерации недоступна: "+err.Error())
		return
	}
	nodes := make([]map[string]any, 0, len(got.GetNodes()))
	for _, n := range got.GetNodes() {
		reasons := make([]map[string]any, 0, len(n.GetReasons()))
		for _, reason := range n.GetReasons() {
			reasons = append(reasons, map[string]any{
				"reason": reason.GetReason(), "weight": reason.GetWeight(),
			})
		}
		nodes = append(nodes, map[string]any{
			"node_id": n.GetNodeId(), "hostname": n.GetHostname(),
			"donor_id": n.GetDonorId(), "trust": n.GetTrust(),
			"payout_reduced": n.GetPayoutReduced(), "payout_stopped": n.GetPayoutStopped(),
			"probe_ok": n.GetProbeOk(), "probe_failed": n.GetProbeFailed(),
			"uptime_pct": n.GetUptimePct(), "last_seen_unix": n.GetLastSeenUnix(),
			"reasons": reasons,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled": true, "nodes": nodes,
		"total": got.GetTotal(), "accused": got.GetAccused(),
	})
}
