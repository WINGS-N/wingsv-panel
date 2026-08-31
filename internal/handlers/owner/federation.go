package owner

import (
	"context"
	"net/http"
	"time"

	"v.wingsnet.org/internal/storage"
)

// headTimeout ограничивает вызов к голове федерации
const headTimeout = 5 * time.Second

type probeVantageView struct {
	ProbeID      string `json:"probe_id"`
	Region       string `json:"region"`
	ISP          string `json:"isp"`
	ASN          string `json:"asn"`
	Version      string `json:"version"`
	Online       bool   `json:"online"`
	LastSeenUnix int64  `json:"last_seen_unix"`
	Measurements uint32 `json:"measurements"`
}

type probeMeasurementView struct {
	NodeID      string `json:"node_id"`
	Hostname    string `json:"hostname"`
	Address     string `json:"address"`
	Transport   string `json:"transport"`
	OK          bool   `json:"ok"`
	HandshakeMs uint32 `json:"handshake_ms"`
	RTTMs       uint32 `json:"rtt_ms"`
	DownloadBps uint64 `json:"download_bps"`
	Error       string `json:"error"`
	ProbeID     string `json:"probe_id"`
	AtUnix      int64  `json:"at_unix"`
}

// handleProbes отдаёт замеры точек наблюдения внутри цензурируемой сети.
// Достижимость из сети головы ничего не говорит о достижимости оттуда, где
// сидят люди, и это единственный экран, где видна разница
func (h *Handler) handleProbes(w http.ResponseWriter, r *http.Request, _ storage.Admin) {
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
	got, err := h.fed.ProbeReports(ctx)
	if err != nil {
		writeError(w, http.StatusBadGateway, "голова федерации недоступна: "+err.Error())
		return
	}
	vantages := make([]probeVantageView, 0, len(got.GetVantages()))
	for _, v := range got.GetVantages() {
		vantages = append(vantages, probeVantageView{
			ProbeID: v.GetProbeId(), Region: v.GetRegion(), ISP: v.GetIsp(), ASN: v.GetAsn(),
			Version: v.GetVersion(), Online: v.GetOnline(),
			LastSeenUnix: v.GetLastSeenUnix(), Measurements: v.GetMeasurements(),
		})
	}
	measurements := make([]probeMeasurementView, 0, len(got.GetMeasurements()))
	for _, m := range got.GetMeasurements() {
		measurements = append(measurements, probeMeasurementView{
			NodeID: m.GetNodeId(), Hostname: m.GetHostname(), Address: m.GetAddress(),
			Transport: m.GetTransport(), OK: m.GetOk(), HandshakeMs: m.GetHandshakeMs(),
			RTTMs: m.GetRttMs(), DownloadBps: m.GetDownloadBps(), Error: m.GetError(),
			ProbeID: m.GetProbeId(), AtUnix: m.GetAtUnix(),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":      true,
		"vantages":     vantages,
		"measurements": measurements,
	})
}

type oracleClassView struct {
	Kind   string  `json:"kind"`
	Count  uint32  `json:"count"`
	Weight float64 `json:"weight"`
}

type oracleSubjectView struct {
	SubjectID        string            `json:"subject_id"`
	Confidence       int32             `json:"confidence"`
	Band             string            `json:"band"`
	Scorer           string            `json:"scorer"`
	AtUnix           int64             `json:"at_unix"`
	Classes          []oracleClassView `json:"classes"`
	ShadowBand       string            `json:"shadow_band"`
	ShadowConfidence int32             `json:"shadow_confidence"`
}

// handleOracle отдаёт состояние судьи. Ни одного поля, по которому профиль
// сводится к человеку: этих данных у головы нет
func (h *Handler) handleOracle(w http.ResponseWriter, r *http.Request, _ storage.Admin) {
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
	got, err := h.fed.OracleOverview(ctx, 20)
	if err != nil {
		writeError(w, http.StatusBadGateway, "голова федерации недоступна: "+err.Error())
		return
	}
	subjects := make([]oracleSubjectView, 0, len(got.GetSubjects()))
	for _, s := range got.GetSubjects() {
		classes := make([]oracleClassView, 0, len(s.GetClasses()))
		for _, c := range s.GetClasses() {
			classes = append(classes, oracleClassView{Kind: c.GetKind(), Count: c.GetCount(), Weight: c.GetWeight()})
		}
		subjects = append(subjects, oracleSubjectView{
			SubjectID: s.GetSubjectId(), Confidence: s.GetConfidence(), Band: s.GetBand(),
			Scorer: s.GetScorer(), AtUnix: s.GetAtUnix(), Classes: classes,
			ShadowBand: s.GetShadowBand(), ShadowConfidence: s.GetShadowConfidence(),
		})
	}
	signals := make([]oracleClassView, 0, len(got.GetSignals()))
	for _, c := range got.GetSignals() {
		signals = append(signals, oracleClassView{Kind: c.GetKind(), Count: c.GetCount(), Weight: c.GetWeight()})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":       true,
		"watched":       got.GetWatched(),
		"full":          got.GetFull(),
		"reduced":       got.GetReduced(),
		"quarantined":   got.GetQuarantined(),
		"subjects":      subjects,
		"signals":       signals,
		"scorer":        got.GetScorer(),
		"shadow_scorer": got.GetShadowScorer(),
	})
}
