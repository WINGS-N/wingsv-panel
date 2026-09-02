package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"v.wingsnet.org/internal/gen/headpb"
	"v.wingsnet.org/internal/storage"
)

// federationTimeout bounds a call to the head. Short: the panel is answering an
// admin who is looking at a page, and a head that is down should say so rather
// than hang the request
const federationTimeout = 5 * time.Second

// donorID is how an admin is known to the federation. The head is told nothing
// else about them: it joins a donor to their nodes and never to a person
func donorID(admin storage.Admin) string {
	return "admin-" + strconv.FormatInt(admin.ID, 10)
}

// federationOn reports whether the operator turned the federation on. Sections
// behind it are not rendered at all rather than rendered disabled, so a
// deployment that never opted in has no federation surface to reason about
func (h *Handler) federationOn() bool {
	if h.fed == nil || !h.fed.Enabled() {
		return false
	}
	value, err := h.store.GetPlatformSetting(storage.SettingFederationEnabled, "")
	if err != nil {
		return false
	}
	return value == "1" || value == "true"
}

// federationSummaryView carries Error so a head that is down shows up as a
// broken panel rather than a missing one
type federationNodeView struct {
	ID                  string  `json:"id"`
	Hostname            string  `json:"hostname"`
	Arch                string  `json:"arch"`
	State               string  `json:"state"`
	Reason              string  `json:"reason"`
	Online              bool    `json:"online"`
	AesNi               bool    `json:"aes_ni"`
	XrayVersion         string  `json:"xray_version"`
	VktpVersion         string  `json:"vktp_version"`
	DeclaredBudgetBytes uint64  `json:"declared_budget_bytes"`
	UsedBytes           uint64  `json:"used_bytes"`
	ProbeBytes          uint64  `json:"probe_bytes"`
	JoinedUnix          int64   `json:"joined_unix"`
	LastSeenUnix        int64   `json:"last_seen_unix"`
	UpRateBps           float64 `json:"up_rate_bps"`
	DownRateBps         float64 `json:"down_rate_bps"`
	Sessions            uint32  `json:"sessions"`
}

// federationMonthView - один закрытый месяц собственного вклада донора
type federationMonthView struct {
	Month string `json:"month"`
	Bytes uint64 `json:"bytes"`
}

type federationSummaryView struct {
	Enabled bool `json:"enabled"`
	// Aggregates only. There is deliberately no field here naming a profile or a
	// user: a donor learns what their machines did, never who did it
	Nodes               uint32  `json:"nodes"`
	NodesOnline         uint32  `json:"nodes_online"`
	Sessions            uint32  `json:"sessions"`
	UpBytes             uint64  `json:"up_bytes"`
	DownBytes           uint64  `json:"down_bytes"`
	UpRateBps           float64 `json:"up_rate_bps"`
	DownRateBps         float64 `json:"down_rate_bps"`
	DeclaredBudgetBytes uint64  `json:"declared_budget_bytes"`
	UsedBytes           uint64  `json:"used_bytes"`
	// ProbeBytes - часть UsedBytes, которую сожгли наши проверки
	ProbeBytes uint64                `json:"probe_bytes"`
	NodeList   []federationNodeView  `json:"node_list"`
	Months     []federationMonthView `json:"months"`
	// Error непустой, когда федерация включена, но голова не отвечает. Раздел
	// при этом остаётся на месте: перезапуск головы не должен выглядеть как
	// отключение федерации
	Error string `json:"error,omitempty"`
}

func (h *Handler) handleFederationSummary(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !h.federationOn() {
		writeJSON(w, http.StatusOK, federationSummaryView{Enabled: false})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), federationTimeout)
	defer cancel()

	donor := donorID(admin)
	summary, err := h.fed.DonorSummary(ctx, donor)
	if err != nil {
		// Голова недоступна - это не повод убирать раздел из меню: федерация
		// включена оператором, и её перезапуск не должен выглядеть как её
		// отключение. Отдаём включённый раздел с пустыми цифрами и текстом
		// ошибки, чтобы человек видел, что именно сломалось.
		writeJSON(w, http.StatusOK, federationSummaryView{
			Enabled: true,
			Error:   "голова федерации недоступна: " + err.Error(),
		})
		return
	}
	nodes, err := h.fed.ListNodes(ctx, donor)
	if err != nil {
		// Голова недоступна - это не повод убирать раздел из меню: федерация
		// включена оператором, и её перезапуск не должен выглядеть как её
		// отключение. Отдаём включённый раздел с пустыми цифрами и текстом
		// ошибки, чтобы человек видел, что именно сломалось.
		writeJSON(w, http.StatusOK, federationSummaryView{
			Enabled: true,
			Error:   "голова федерации недоступна: " + err.Error(),
		})
		return
	}

	view := federationSummaryView{
		Enabled:             true,
		Nodes:               summary.GetNodes(),
		NodesOnline:         summary.GetNodesOnline(),
		Sessions:            summary.GetSessions(),
		UpBytes:             summary.GetUpBytes(),
		DownBytes:           summary.GetDownBytes(),
		UpRateBps:           summary.GetUpRateBps(),
		DownRateBps:         summary.GetDownRateBps(),
		DeclaredBudgetBytes: summary.GetDeclaredBudgetBytes(),
		UsedBytes:           summary.GetUsedBytes(),
		ProbeBytes:          summary.GetProbeBytes(),
	}
	// История - не повод завалить экран: голова без неё отдаёт Unimplemented,
	// и раздел просто останется без графика
	if history, err := h.fed.DonorHistory(ctx, donor, 12); err == nil {
		for _, m := range history {
			view.Months = append(view.Months, federationMonthView{
				Month: m.GetMonth(), Bytes: m.GetBytes(),
			})
		}
	}
	for _, n := range nodes.GetNodes() {
		view.NodeList = append(view.NodeList, federationNodeView{
			ID:                  n.GetNodeId(),
			Hostname:            n.GetHostname(),
			Arch:                n.GetArch(),
			State:               n.GetState(),
			Reason:              n.GetReason(),
			Online:              n.GetOnline(),
			AesNi:               n.GetAesNi(),
			XrayVersion:         n.GetXrayVersion(),
			VktpVersion:         n.GetVktpVersion(),
			DeclaredBudgetBytes: n.GetDeclaredBudgetBytes(),
			UsedBytes:           n.GetUsedBytes(),
			ProbeBytes:          n.GetProbeBytes(),
			JoinedUnix:          n.GetJoinedUnix(),
			LastSeenUnix:        n.GetLastSeenUnix(),
			UpRateBps:           n.GetLive().GetUpRateBps(),
			DownRateBps:         n.GetLive().GetDownRateBps(),
			Sessions:            n.GetLive().GetSessions(),
		})
	}
	writeJSON(w, http.StatusOK, view)
}

type enrollTokenView struct {
	Token       string `json:"token"`
	ExpiresUnix int64  `json:"expires_unix"`
	// Command is what the donor pastes. Built here rather than in the browser so
	// the public base URL is the one the panel actually serves on
	Command string `json:"command"`
	// Uses is how many nodes may still join on this token. One for the ordinary
	// paste-one-command donor; more for a fleet enrolling from one Secret.
	Uses uint32 `json:"uses"`
}

func (h *Handler) handleFederationEnrollToken(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !h.federationOn() {
		writeError(w, http.StatusNotFound, "federation is off")
		return
	}
	var req struct {
		TTLMinutes int    `json:"ttl_minutes"`
		Uses       uint32 `json:"uses"`
		// BudgetGB - месячный потолок отдаваемой машины. Ноль оставляет общий
		BudgetGB uint32 `json:"budget_gb"`
	}
	// An empty body is fine: the head applies its own default
	_ = json.NewDecoder(r.Body).Decode(&req)

	ctx, cancel := context.WithTimeout(r.Context(), federationTimeout)
	defer cancel()
	got, err := h.fed.MintEnrollToken(
		ctx, donorID(admin), time.Duration(req.TTLMinutes)*time.Minute, req.Uses, req.BudgetGB,
	)
	if err != nil {
		writeError(w, http.StatusBadGateway, "federation head unreachable: "+err.Error())
		return
	}
	// The head builds the command: it is what serves the installer and knows its
	// own public address. The panel only falls back when the head serves none,
	// and then it says so rather than inventing a URL
	command := got.GetInstallCommand()
	if command == "" {
		command = "the federation head is not serving an installer; set its -sub-base and -agent-release"
	}
	writeJSON(w, http.StatusOK, enrollTokenView{
		Token:       got.GetEnrollToken(),
		ExpiresUnix: got.GetExpiresUnix(),
		Command:     command,
		Uses:        got.GetUses(),
	})
}

func (h *Handler) handleFederationNodeState(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !h.federationOn() {
		writeError(w, http.StatusNotFound, "federation is off")
		return
	}
	// Один обработчик на два адреса: панель ходит сессией, телефон - токеном
	// устройства, а делают они ровно одно и то же
	path := r.URL.Path
	for _, prefix := range []string{"/api/admin/federation/nodes/", "/api/app/federation/nodes/"} {
		path = strings.TrimPrefix(path, prefix)
	}
	nodeID, action, _ := strings.Cut(path, "/")
	if nodeID == "" {
		writeError(w, http.StatusBadRequest, "missing node id")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), federationTimeout)
	defer cancel()
	// The node has to belong to this admin. The head checks no ownership here, so
	// a panel that skipped this would let any admin touch anyone's machine
	owned, err := h.fed.ListNodes(ctx, donorID(admin))
	if err != nil {
		writeError(w, http.StatusBadGateway, "federation head unreachable: "+err.Error())
		return
	}
	var mine bool
	for _, n := range owned.GetNodes() {
		if n.GetNodeId() == nodeID {
			mine = true
			break
		}
	}
	if !mine {
		writeError(w, http.StatusNotFound, "no such node")
		return
	}

	switch action {
	case "state":
		var req struct {
			State  string `json:"state"`
			Reason string `json:"reason"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "bad request body")
			return
		}
		if err := h.fed.SetNodeState(ctx, nodeID, req.State, req.Reason); err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case "budget":
		var req struct {
			DeclaredBudgetBytes uint64 `json:"declared_budget_bytes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "bad request body")
			return
		}
		got, err := h.fed.SetNodeBudget(ctx, nodeID, req.DeclaredBudgetBytes)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"declared_budget_bytes": got})
	default:
		writeError(w, http.StatusNotFound, "no such node action")
	}
}

// handleFederationLive отдаёт потоком собственные счётчики донора. Цифры всего
// флота идут в общий admin-сокет, и им место на лендинге, а не на странице
// "ваши ноды"
func (h *Handler) handleFederationLive(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	if !h.federationOn() {
		writeError(w, http.StatusNotFound, "federation is off")
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	frames := make(chan []byte, 1)
	go h.fed.StreamDonor(r.Context(), donorID(admin), func(update *headpb.LiveUpdate) {
		d := update.GetDonor()
		payload, err := json.Marshal(map[string]any{
			"nodes":         d.GetNodes(),
			"nodes_online":  d.GetNodesOnline(),
			"sessions":      d.GetSessions(),
			"up_rate_bps":   d.GetUpRateBps(),
			"down_rate_bps": d.GetDownRateBps(),
			"used_bytes":    d.GetUsedBytes(),
		})
		if err != nil {
			return
		}
		select {
		case <-frames:
		default:
		}
		frames <- payload
	})

	for {
		select {
		case <-r.Context().Done():
			return
		case payload := <-frames:
			if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
