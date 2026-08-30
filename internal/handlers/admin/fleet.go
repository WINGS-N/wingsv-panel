package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	headpb "v.wingsnet.org/internal/gen/headpb"
	"v.wingsnet.org/internal/storage"
)

// xrayForkRepo is where fleet builds come from, and it is not a setting.
//
// Only our fork carries the WINGS patches - per-peer WireGuard stats and the
// gVisor TUN uid filter - and a node running upstream Xray would look healthy
// while the numbers the whole budget rests on stayed empty. Letting an operator
// point this anywhere would make that a click away.
const xrayForkRepo = "WINGS-N/Xray-core"

// relayForkRepo is the matching source for the relay
const relayForkRepo = "WINGS-N/vk-turn-proxy"

type fleetView struct {
	XrayVersion   string `json:"xray_version"`
	XrayURL       string `json:"xray_url"`
	XraySHA512    string `json:"xray_sha512"`
	VKTPVersion   string `json:"vktp_version"`
	VKTPURL       string `json:"vktp_url"`
	VKTPSHA512    string `json:"vktp_sha512"`
	AutoUpgrade   bool   `json:"auto_upgrade"`
	RealityDest   string `json:"reality_dest"`
	AutoDest      bool   `json:"auto_dest"`
	PostQuantum   bool   `json:"post_quantum"`
	TCPPort       uint32 `json:"tcp_port"`
	XHTTPPort     uint32 `json:"xhttp_port"`
	ConfigVersion uint64 `json:"config_version"`
	DestPoolSize  uint32 `json:"dest_pool_size"`
}

func (h *Handler) handleFleetSettings(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	if admin.Role != storage.RoleOwner {
		writeError(w, http.StatusForbidden, "версией флота распоряжается владелец")
		return
	}
	if !h.federationOn() {
		writeError(w, http.StatusNotFound, "federation is off")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), federationTimeout)
	defer cancel()

	switch r.Method {
	case http.MethodGet:
		got, err := h.fed.FleetSettings(ctx)
		if err != nil {
			writeError(w, http.StatusBadGateway, "голова федерации недоступна: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, toFleetView(got))

	case http.MethodPost:
		var req fleetView
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "bad json")
			return
		}
		// Сборки принимаем только из наших форков: чужой билд Xray не несёт
		// патчей, на которых стоит весь учёт трафика
		if err := fromOurFork(req.XrayURL, xrayForkRepo); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := fromOurFork(req.VKTPURL, relayForkRepo); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		saved, err := h.fed.SetFleetSettings(ctx, &headpb.FleetSettings{
			Xray:        &headpb.BuildChoice{Version: req.XrayVersion, Url: req.XrayURL, Sha512: req.XraySHA512},
			Vktp:        &headpb.BuildChoice{Version: req.VKTPVersion, Url: req.VKTPURL, Sha512: req.VKTPSHA512},
			AutoUpgrade: req.AutoUpgrade,
			RealityDest: req.RealityDest,
			AutoDest:    req.AutoDest,
			PostQuantum: req.PostQuantum,
			TcpPort:     req.TCPPort,
			XhttpPort:   req.XHTTPPort,
		})
		if err != nil {
			writeError(w, http.StatusBadGateway, "голова федерации не приняла настройки: "+err.Error())
			return
		}
		_ = h.store.AppendAudit(storage.AuditEntry{
			ActorAdminID: admin.ID, ActorUsername: admin.Username,
			Action: "owner.fleet_settings", TargetType: "fleet", Message: req.XrayVersion,
		})
		writeJSON(w, http.StatusOK, toFleetView(saved))

	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *Handler) handleFleetRestart(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if admin.Role != storage.RoleOwner {
		writeError(w, http.StatusForbidden, "перезапуском флота распоряжается владелец")
		return
	}
	var req struct {
		Component string `json:"component"`
		NodeID    string `json:"node_id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	ctx, cancel := context.WithTimeout(r.Context(), federationTimeout)
	defer cancel()
	n, err := h.fed.RestartComponent(ctx, req.Component, req.NodeID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "не удалось разослать команду: "+err.Error())
		return
	}
	_ = h.store.AppendAudit(storage.AuditEntry{
		ActorAdminID: admin.ID, ActorUsername: admin.Username,
		Action: "owner.fleet_restart", TargetType: "fleet", TargetID: req.NodeID, Message: req.Component,
	})
	writeJSON(w, http.StatusOK, map[string]any{"nodes": n})
}

// fromOurFork refuses a build that did not come from the fork we maintain.
// Checked here rather than trusted from the browser: this endpoint is what
// decides which binary a few dozen donated machines will execute.
func fromOurFork(rawURL, repo string) error {
	if strings.TrimSpace(rawURL) == "" {
		return nil
	}
	prefix := "https://github.com/" + repo + "/releases/download/"
	if !strings.HasPrefix(rawURL, prefix) {
		return errRejected(repo)
	}
	return nil
}

func errRejected(repo string) error {
	return &fleetError{msg: "сборки берём только из " + repo + ": чужой билд не несёт патчей WINGS"}
}

type fleetError struct{ msg string }

func (e *fleetError) Error() string { return e.msg }

func toFleetView(s *headpb.FleetSettings) fleetView {
	return fleetView{
		XrayVersion:   s.GetXray().GetVersion(),
		XrayURL:       s.GetXray().GetUrl(),
		XraySHA512:    s.GetXray().GetSha512(),
		VKTPVersion:   s.GetVktp().GetVersion(),
		VKTPURL:       s.GetVktp().GetUrl(),
		VKTPSHA512:    s.GetVktp().GetSha512(),
		AutoUpgrade:   s.GetAutoUpgrade(),
		RealityDest:   s.GetRealityDest(),
		AutoDest:      s.GetAutoDest(),
		PostQuantum:   s.GetPostQuantum(),
		TCPPort:       s.GetTcpPort(),
		XHTTPPort:     s.GetXhttpPort(),
		ConfigVersion: s.GetConfigVersion(),
		DestPoolSize:  s.GetDestPoolSize(),
	}
}

// handleFleetReleases lists what our fork has published, with the asset a Linux
// node actually needs.
//
// The panel offers a choice rather than a text box: an operator typing a URL and
// a digest by hand is how a fleet ends up running a build nobody can identify.
func (h *Handler) handleFleetReleases(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	if admin.Role != storage.RoleOwner {
		writeError(w, http.StatusForbidden, "версией флота распоряжается владелец")
		return
	}
	repo := xrayForkRepo
	if r.URL.Query().Get("component") == "vktp" {
		repo = relayForkRepo
	}
	ctx, cancel := context.WithTimeout(r.Context(), federationTimeout)
	defer cancel()

	releases, err := fetchReleases(ctx, repo)
	if err != nil {
		writeError(w, http.StatusBadGateway, "не удалось получить список релизов: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"repo": repo, "releases": releases})
}

type releaseView struct {
	Tag         string `json:"tag"`
	PublishedAt string `json:"published_at"`
	// AssetURL is the linux-amd64 build; empty when a release has none, which is
	// shown rather than hidden so nobody wonders why a tag cannot be selected
	AssetURL   string `json:"asset_url"`
	Prerelease bool   `json:"prerelease"`
}

func fetchReleases(ctx context.Context, repo string) ([]releaseView, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.github.com/repos/"+repo+"/releases?per_page=20", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, errRejected(repo + " (github " + resp.Status + ")")
	}

	var raw []struct {
		TagName     string `json:"tag_name"`
		PublishedAt string `json:"published_at"`
		Prerelease  bool   `json:"prerelease"`
		Assets      []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	out := make([]releaseView, 0, len(raw))
	for _, rel := range raw {
		view := releaseView{Tag: rel.TagName, PublishedAt: rel.PublishedAt, Prerelease: rel.Prerelease}
		for _, a := range rel.Assets {
			// Имя ассета - контракт с binfetch на стороне агента
			if a.Name == "Xray-linux-64.zip" || strings.HasSuffix(a.Name, "linux-amd64") {
				view.AssetURL = a.URL
				break
			}
		}
		out = append(out, view)
	}
	return out, nil
}

type fleetNodeView struct {
	ID                  string `json:"id"`
	DonorID             string `json:"donor_id"`
	Hostname            string `json:"hostname"`
	State               string `json:"state"`
	Reason              string `json:"reason"`
	Online              bool   `json:"online"`
	Arch                string `json:"arch"`
	LastSeenUnix        int64  `json:"last_seen_unix"`
	DeclaredBudgetBytes uint64 `json:"declared_budget_bytes"`
	UsedBytes           uint64 `json:"used_bytes"`
	// Порты и цель у каждой ноды свои - показываем их, а не значения флота:
	// иначе панель уверяет, что все сидят на одной паре портов
	OfferedPorts []uint32 `json:"offered_ports"`
	RealityDest  string   `json:"reality_dest"`
	// Mine отделяет свои ноды от чужих в общем списке владельца: он видит весь
	// флот, но должен различать, за что отвечает сам
	Mine bool `json:"mine"`
}

// handleFleetNodes lists nodes. The owner sees the whole federation, everybody
// else only their own - a donor has no business enumerating other people's
// machines, and that is a decision made here rather than in the browser.
func (h *Handler) handleFleetNodes(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	if !h.federationOn() {
		writeError(w, http.StatusNotFound, "federation is off")
		return
	}
	mine := donorID(admin)
	scope := mine
	if admin.Role == storage.RoleOwner {
		scope = ""
	}
	ctx, cancel := context.WithTimeout(r.Context(), federationTimeout)
	defer cancel()

	resp, err := h.fed.ListNodes(ctx, scope)
	if err != nil {
		writeError(w, http.StatusBadGateway, "голова федерации недоступна: "+err.Error())
		return
	}
	out := make([]fleetNodeView, 0, len(resp.GetNodes()))
	for _, n := range resp.GetNodes() {
		out = append(out, fleetNodeView{
			ID:                  n.GetNodeId(),
			DonorID:             n.GetDonorId(),
			Hostname:            n.GetHostname(),
			State:               n.GetState(),
			Reason:              n.GetReason(),
			Online:              n.GetOnline(),
			Arch:                n.GetArch(),
			LastSeenUnix:        n.GetLastSeenUnix(),
			DeclaredBudgetBytes: n.GetDeclaredBudgetBytes(),
			UsedBytes:           n.GetUsedBytes(),
			OfferedPorts:        n.GetOfferedPorts(),
			RealityDest:         n.GetRealityDest(),
			Mine:                n.GetDonorId() == mine,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"nodes": out, "all": scope == ""})
}
