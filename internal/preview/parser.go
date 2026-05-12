package preview

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"google.golang.org/protobuf/proto"

	"v.wingsnet.org/internal/gen/wingsvpb"
)

const (
	SchemePrefix          = "wingsv://"
	FormatProtobufDeflate = 0x12
)

type Preview struct {
	RawLink       string           `json:"rawLink"`
	LinkType      string           `json:"linkType"`
	Backend       string           `json:"backend"`
	ConfigType    string           `json:"configType"`
	Version       uint32           `json:"version"`
	Title         string           `json:"title"`
	Subtitle      string           `json:"subtitle"`
	ProfilesCount int              `json:"profilesCount"`
	Subscriptions int              `json:"subscriptionsCount"`
	ProfileTitles []string         `json:"profileTitles"`
	QuickFacts    []PreviewFact    `json:"quickFacts"`
	Sections      []PreviewSection `json:"sections,omitempty"`
}

type PreviewFact struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

type PreviewSection struct {
	Title string        `json:"title"`
	Note  string        `json:"note,omitempty"`
	Facts []PreviewFact `json:"facts,omitempty"`
	Items []string      `json:"items,omitempty"`
}

func Parse(raw string) (*Preview, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("empty link")
	}
	if strings.HasPrefix(strings.ToLower(raw), "vless://") {
		return parseVLESS(raw)
	}
	if !strings.HasPrefix(strings.ToLower(raw), SchemePrefix) {
		return nil, errors.New("unsupported link scheme")
	}
	return parseWings(raw)
}

func parseVLESS(raw string) (*Preview, error) {
	uri, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	query := uri.Query()
	host := uri.Hostname()
	port := uri.Port()
	title := host
	if title == "" {
		title = "VLESS профиль"
	}
	if fragment := strings.TrimSpace(uri.Fragment); fragment != "" {
		title, _ = url.QueryUnescape(fragment)
	}

	subtitle := host
	if port != "" {
		subtitle = fmt.Sprintf("%s:%s", host, port)
	}

	quickFacts := []PreviewFact{
		{Label: "Тип", Value: "VLESS"},
		{Label: "Адрес", Value: subtitle},
	}
	transportFacts := []PreviewFact{}
	appendFact := func(facts *[]PreviewFact, label, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		*facts = append(*facts, PreviewFact{Label: label, Value: value})
	}

	appendFact(&transportFacts, "Транспорт", firstNonEmpty(query.Get("type"), "tcp"))
	appendFact(&transportFacts, "Безопасность", firstNonEmpty(query.Get("security"), "none"))
	appendFact(&transportFacts, "SNI", query.Get("sni"))
	appendFact(&transportFacts, "Host", query.Get("host"))
	appendFact(&transportFacts, "Path", query.Get("path"))
	appendFact(&transportFacts, "Service", query.Get("serviceName"))
	appendFact(&transportFacts, "Flow", query.Get("flow"))
	appendFact(&transportFacts, "ALPN", query.Get("alpn"))
	appendFact(&transportFacts, "Fingerprint", query.Get("fp"))

	preview := &Preview{
		RawLink:       raw,
		LinkType:      "vless",
		Backend:       "Xray",
		ConfigType:    "VLESS",
		Version:       1,
		Title:         title,
		Subtitle:      subtitle,
		ProfilesCount: 1,
		ProfileTitles: []string{title},
		QuickFacts:    quickFacts,
	}
	if len(transportFacts) > 0 {
		preview.Sections = []PreviewSection{{Title: "Транспорт", Facts: transportFacts}}
	}
	return preview, nil
}

func parseWings(raw string) (*Preview, error) {
	payload := strings.TrimPrefix(raw, SchemePrefix)
	payload = normalizeBase64(payload)
	decoded, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		decoded, err = base64.URLEncoding.DecodeString(payload)
		if err != nil {
			return nil, err
		}
	}
	if len(decoded) == 0 {
		return nil, errors.New("empty payload")
	}
	if decoded[0] != FormatProtobufDeflate {
		return nil, errors.New("unsupported payload format")
	}

	protobufPayload, err := inflatePayload(decoded[1:])
	if err != nil {
		return nil, err
	}

	config := &wingsvpb.Config{}
	if err := proto.Unmarshal(protobufPayload, config); err != nil {
		return nil, err
	}
	return buildPreview(raw, config), nil
}

func inflatePayload(payload []byte) ([]byte, error) {
	if len(payload) == 0 {
		return nil, errors.New("empty compressed payload")
	}

	zlibReader, err := zlib.NewReader(bytes.NewReader(payload))
	if err == nil {
		defer zlibReader.Close()
		return io.ReadAll(zlibReader)
	}

	flateReader := flate.NewReader(bytes.NewReader(payload))
	defer flateReader.Close()

	inflated, flateErr := io.ReadAll(flateReader)
	if flateErr != nil {
		return nil, fmt.Errorf("ссылка повреждена или обрезана: %w", flateErr)
	}
	return inflated, nil
}

func buildPreview(raw string, config *wingsvpb.Config) *Preview {
	preview := &Preview{
		RawLink:       raw,
		LinkType:      "wingsv",
		Backend:       backendLabel(config.GetBackend()),
		ConfigType:    configTypeLabel(config.GetType()),
		Version:       config.GetVer(),
		Title:         "WINGS V ссылка",
		Subtitle:      "Сведения о конфигурации",
		ProfilesCount: len(config.GetXray().GetProfiles()),
		Subscriptions: len(config.GetXray().GetSubscriptions()),
	}

	preview.QuickFacts = append(preview.QuickFacts,
		PreviewFact{Label: "Backend", Value: preview.Backend},
		PreviewFact{Label: "Тип", Value: preview.ConfigType},
	)
	if preview.Version > 0 {
		preview.QuickFacts = append(preview.QuickFacts, PreviewFact{
			Label: "Версия",
			Value: fmt.Sprintf("v%d", preview.Version),
		})
	}
	if preview.ProfilesCount > 0 {
		preview.QuickFacts = append(preview.QuickFacts, PreviewFact{
			Label: "Xray профили",
			Value: fmt.Sprintf("%d", preview.ProfilesCount),
		})
	}
	if preview.Subscriptions > 0 {
		preview.QuickFacts = append(preview.QuickFacts, PreviewFact{
			Label: "Подписки",
			Value: fmt.Sprintf("%d", preview.Subscriptions),
		})
	}

	if turnSection := buildTurnSection(config.GetTurn()); turnSection != nil {
		preview.Sections = append(preview.Sections, *turnSection)
	}
	if xraySection := buildXraySettingsSection(config.GetXray()); xraySection != nil {
		preview.Sections = append(preview.Sections, *xraySection)
	}
	if profilesSection := buildXrayProfilesSection(config.GetXray()); profilesSection != nil {
		preview.Sections = append(preview.Sections, *profilesSection)
	}
	if subsSection := buildXraySubscriptionsSection(config.GetXray()); subsSection != nil {
		preview.Sections = append(preview.Sections, *subsSection)
	}
	if routingSection := buildXrayRoutingSection(config.GetXray().GetRouting()); routingSection != nil {
		preview.Sections = append(preview.Sections, *routingSection)
	}
	if wgSection := buildWireGuardSection(config.GetWg()); wgSection != nil {
		preview.Sections = append(preview.Sections, *wgSection)
	}
	if awgSection := buildAmneziaSection(config.GetAwg()); awgSection != nil {
		preview.Sections = append(preview.Sections, *awgSection)
	}
	if appSection := buildAppRoutingSection(config.GetAppRouting()); appSection != nil {
		preview.Sections = append(preview.Sections, *appSection)
	}
	if wbSection := buildWbStreamSection(config.GetWbStream()); wbSection != nil {
		preview.Sections = append(preview.Sections, *wbSection)
	}
	if xposedSection := buildXposedSection(config.GetXposed()); xposedSection != nil {
		preview.Sections = append(preview.Sections, *xposedSection)
	}
	if rootSection := buildRootSettingsSection(config.GetRoot()); rootSection != nil {
		preview.Sections = append(preview.Sections, *rootSection)
	}
	if appPrefsSection := buildAppPreferencesSection(config.GetAppPreferences()); appPrefsSection != nil {
		preview.Sections = append(preview.Sections, *appPrefsSection)
	}
	if guardianSection := buildGuardianSection(config.GetGuardian()); guardianSection != nil {
		// Push Guardian to the top so the warning is the first thing the user sees.
		preview.Sections = append([]PreviewSection{*guardianSection}, preview.Sections...)
		if host := guardianHost(config.GetGuardian().GetWsUrl()); host != "" {
			preview.QuickFacts = append(preview.QuickFacts, PreviewFact{
				Label: "Попечитель",
				Value: host,
			})
		}
	}
	if hwidSection := buildSubscriptionHwidSection(config.GetSubscriptionHwid()); hwidSection != nil {
		preview.Sections = append(preview.Sections, *hwidSection)
	}
	if sharingSection := buildSharingSection(config.GetSharing()); sharingSection != nil {
		preview.Sections = append(preview.Sections, *sharingSection)
	}
	if byeDpiSection := buildByeDpiSection(config.GetByeDpi()); byeDpiSection != nil {
		preview.Sections = append(preview.Sections, *byeDpiSection)
	}

	titleSetByConfigType := false
	switch config.GetType() {
	case wingsvpb.ConfigType_CONFIG_TYPE_ALL:
		preview.Title = "Полная конфигурация"
		if preview.Backend != "" && preview.Backend != "WINGS V" {
			preview.Subtitle = preview.Backend
		} else {
			preview.Subtitle = "Все настройки приложения"
		}
		titleSetByConfigType = true
	case wingsvpb.ConfigType_CONFIG_TYPE_APP_ROUTING:
		preview.Title = "Per-app routing"
		if app := config.GetAppRouting(); app != nil {
			mode := boolLabel(app.GetBypass(), "Bypass", "Только выбранные")
			pkgCount := len(app.GetPackages())
			preview.Subtitle = fmt.Sprintf("%s · %d приложений", mode, pkgCount)
		}
		titleSetByConfigType = true
	case wingsvpb.ConfigType_CONFIG_TYPE_XRAY_ROUTING:
		preview.Title = "Xray routing"
		if routing := config.GetXray().GetRouting(); routing != nil {
			preview.Subtitle = fmt.Sprintf("%d правил", len(routing.GetRules()))
		}
		titleSetByConfigType = true
	case wingsvpb.ConfigType_CONFIG_TYPE_GUARDIAN:
		preview.Title = "Подключение к попечителю"
		host := guardianHost(config.GetGuardian().GetWsUrl())
		if host == "" {
			host = "панель не указана"
		}
		preview.Subtitle = host
		titleSetByConfigType = true
	}

	profiles := config.GetXray().GetProfiles()
	if len(profiles) > 0 {
		if !titleSetByConfigType {
			preview.Title = strings.TrimSpace(profiles[0].GetTitle())
			if preview.Title == "" {
				preview.Title = "Xray профиль"
			}
			subtitle := strings.TrimSpace(profiles[0].GetAddress())
			if profiles[0].GetPort() > 0 {
				subtitle = fmt.Sprintf("%s:%d", subtitle, profiles[0].GetPort())
			}
			if subtitle != "" {
				preview.Subtitle = subtitle
			}
		}
		for _, profile := range profiles {
			title := strings.TrimSpace(profile.GetTitle())
			if title == "" {
				title = strings.TrimSpace(profile.GetAddress())
			}
			if title != "" {
				preview.ProfileTitles = append(preview.ProfileTitles, title)
			}
		}
	} else if !titleSetByConfigType {
		if wb := config.GetWbStream(); wb != nil && strings.TrimSpace(wb.GetRoomId()) != "" {
			preview.Title = "WB Stream"
			preview.Subtitle = "Room " + strings.TrimSpace(wb.GetRoomId())
		} else if turn := config.GetTurn(); endpointLabel(turn.GetEndpoint()) != "" {
			preview.Title = "VK TURN"
			preview.Subtitle = endpointLabel(turn.GetEndpoint())
		} else if subs := config.GetXray().GetSubscriptions(); len(subs) > 0 {
			preview.Title = "Подписка Xray"
			preview.Subtitle = strings.TrimSpace(subs[0].GetTitle())
		} else if preview.Backend != "" {
			preview.Title = preview.Backend
		}
	}

	return preview
}

func buildTurnSection(turn *wingsvpb.Turn) *PreviewSection {
	if turn == nil {
		return nil
	}
	endpoint := endpointLabel(turn.GetEndpoint())
	link := strings.TrimSpace(turn.GetLink())
	links := turn.GetLinks()
	secondary := strings.TrimSpace(turn.GetLinkSecondary())
	host := strings.TrimSpace(turn.GetHost())
	port := turn.GetPort()
	local := endpointLabel(turn.GetLocalEndpoint())
	sessionMode := turnSessionModeLabel(turn.GetSessionMode())
	allLinks := append([]string{}, links...)
	if link != "" {
		found := false
		for _, l := range allLinks {
			if l == link {
				found = true
				break
			}
		}
		if !found {
			allLinks = append([]string{link}, allLinks...)
		}
	}
	if endpoint == "" && len(allLinks) == 0 && secondary == "" && host == "" && port == 0 && local == "" && sessionMode == "" {
		return nil
	}

	section := PreviewSection{Title: "VK TURN"}
	appendFact := func(label, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		section.Facts = append(section.Facts, PreviewFact{Label: label, Value: value})
	}
	if endpoint != "" {
		appendFact("Endpoint", endpoint)
	}
	if local != "" {
		appendFact("Local endpoint", local)
	}
	if host != "" {
		hostLabel := host
		if port > 0 {
			hostLabel = fmt.Sprintf("%s:%d", host, port)
		}
		appendFact("Host", hostLabel)
	} else if port > 0 {
		appendFact("Port", fmt.Sprintf("%d", port))
	}
	if turn.Threads != nil {
		appendFact("Воркеров", fmt.Sprintf("%d", turn.GetThreads()))
	}
	if turn.CredsGroupSize != nil {
		appendFact("Размер группы кредов", fmt.Sprintf("%d", turn.GetCredsGroupSize()))
	}
	if sessionMode != "" {
		appendFact("Сессия", sessionMode)
	}
	if turn.UseUdp != nil {
		appendFact("Транспорт", boolLabel(turn.GetUseUdp(), "UDP", "TCP"))
	}
	if turn.NoObfuscation != nil {
		appendFact("Обфускация", boolLabel(turn.GetNoObfuscation(), "Выкл", "Вкл"))
	}
	if turn.ManualCaptcha != nil {
		appendFact("Captcha", boolLabel(turn.GetManualCaptcha(), "Ручная", "Авто"))
	}
	if solver := strings.TrimSpace(turn.GetCaptchaAutoSolver()); solver != "" {
		appendFact("Captcha solver", solver)
	}
	if turn.RestartOnNetworkChange != nil {
		appendFact("Перезапуск при смене сети", boolLabel(turn.GetRestartOnNetworkChange(), "Да", "Нет"))
	}
	if mode := proxyRuntimeModeLabel(turn.GetRuntimeMode()); mode != "" {
		appendFact("Режим runtime", mode)
	}
	if secondary != "" {
		appendFact("Резервная ссылка", secondary)
	}
	if len(allLinks) > 0 {
		appendFact("VK ссылок", fmt.Sprintf("%d", len(allLinks)))
		section.Items = allLinks
	}
	return &section
}

func buildXraySettingsSection(x *wingsvpb.Xray) *PreviewSection {
	if x == nil {
		return nil
	}
	settings := x.GetSettings()
	if settings == nil {
		return nil
	}
	section := PreviewSection{Title: "Настройки Xray"}
	appendFact := func(label, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		section.Facts = append(section.Facts, PreviewFact{Label: label, Value: value})
	}
	if mode := xrayTransportModeLabel(settings.GetTransportMode()); mode != "" {
		appendFact("Транспорт", mode)
	}
	if settings.LocalProxyEnabled != nil {
		port := settings.GetLocalProxyPort()
		value := boolLabel(settings.GetLocalProxyEnabled(), "Вкл", "Выкл")
		if settings.GetLocalProxyEnabled() && port > 0 {
			value = fmt.Sprintf("Вкл · :%d", port)
		}
		appendFact("Local proxy", value)
	} else if port := settings.GetLocalProxyPort(); port > 0 {
		appendFact("Local proxy", fmt.Sprintf(":%d", port))
	}
	if settings.LocalProxyAuthEnabled != nil && settings.GetLocalProxyAuthEnabled() {
		appendFact("Local proxy auth", "Вкл")
	}
	if remote := strings.TrimSpace(settings.GetRemoteDns()); remote != "" {
		appendFact("Remote DNS", remote)
	}
	if direct := strings.TrimSpace(settings.GetDirectDns()); direct != "" {
		appendFact("Direct DNS", direct)
	}
	if settings.AllowLan != nil {
		appendFact("Allow LAN", boolLabel(settings.GetAllowLan(), "Вкл", "Выкл"))
	}
	if settings.AllowInsecure != nil {
		appendFact("Allow insecure", boolLabel(settings.GetAllowInsecure(), "Вкл", "Выкл"))
	}
	if settings.Ipv6 != nil {
		appendFact("IPv6", boolLabel(settings.GetIpv6(), "Вкл", "Выкл"))
	}
	if settings.SniffingEnabled != nil {
		appendFact("Sniffing", boolLabel(settings.GetSniffingEnabled(), "Вкл", "Выкл"))
	}
	if settings.ProxyQuicEnabled != nil {
		appendFact("Proxy QUIC", boolLabel(settings.GetProxyQuicEnabled(), "Вкл", "Выкл"))
	}
	if settings.RestartOnNetworkChange != nil {
		appendFact("Перезапуск при смене сети", boolLabel(settings.GetRestartOnNetworkChange(), "Вкл", "Выкл"))
	}
	if x.MergeOnly != nil && x.GetMergeOnly() {
		appendFact("Только дополнение", "Да")
	}
	if len(section.Facts) == 0 {
		return nil
	}
	return &section
}

func buildXrayProfilesSection(x *wingsvpb.Xray) *PreviewSection {
	profiles := x.GetProfiles()
	if len(profiles) == 0 {
		return nil
	}
	section := PreviewSection{Title: "Xray профили"}
	for _, profile := range profiles {
		title := strings.TrimSpace(profile.GetTitle())
		if title == "" {
			title = strings.TrimSpace(profile.GetAddress())
		}
		address := strings.TrimSpace(profile.GetAddress())
		if profile.GetPort() > 0 && address != "" {
			address = fmt.Sprintf("%s:%d", address, profile.GetPort())
		}
		if address != "" && title != address {
			title = fmt.Sprintf("%s · %s", title, address)
		} else if title == "" && address != "" {
			title = address
		}
		if title == "" {
			continue
		}
		section.Items = append(section.Items, title)
	}
	if len(section.Items) == 0 {
		return nil
	}
	return &section
}

func buildXraySubscriptionsSection(x *wingsvpb.Xray) *PreviewSection {
	subs := x.GetSubscriptions()
	if len(subs) == 0 {
		return nil
	}
	section := PreviewSection{Title: "Подписки Xray"}
	for _, sub := range subs {
		title := strings.TrimSpace(sub.GetTitle())
		urlLabel := strings.TrimSpace(sub.GetUrl())
		if title == "" {
			title = urlLabel
		} else if urlLabel != "" && title != urlLabel {
			title = fmt.Sprintf("%s · %s", title, urlLabel)
		}
		if title == "" {
			continue
		}
		section.Items = append(section.Items, title)
	}
	if len(section.Items) == 0 {
		return nil
	}
	return &section
}

func buildXrayRoutingSection(routing *wingsvpb.XrayRouting) *PreviewSection {
	if routing == nil {
		return nil
	}
	rules := routing.GetRules()
	if len(rules) == 0 && strings.TrimSpace(routing.GetGeoipUrl()) == "" && strings.TrimSpace(routing.GetGeositeUrl()) == "" {
		return nil
	}
	section := PreviewSection{Title: "Xray routing"}
	if len(rules) > 0 {
		enabled := 0
		for _, rule := range rules {
			if rule.Enabled == nil || rule.GetEnabled() {
				enabled++
			}
		}
		section.Facts = append(section.Facts, PreviewFact{
			Label: "Правила",
			Value: fmt.Sprintf("%d (активных %d)", len(rules), enabled),
		})
	}
	if geoip := strings.TrimSpace(routing.GetGeoipUrl()); geoip != "" {
		section.Facts = append(section.Facts, PreviewFact{Label: "GeoIP", Value: geoip})
	}
	if geosite := strings.TrimSpace(routing.GetGeositeUrl()); geosite != "" {
		section.Facts = append(section.Facts, PreviewFact{Label: "Geosite", Value: geosite})
	}
	return &section
}

func buildWireGuardSection(wg *wingsvpb.WireGuard) *PreviewSection {
	if wg == nil {
		return nil
	}
	endpoint := endpointLabel(wg.GetEndpoint())
	iface := wg.GetIface()
	peer := wg.GetPeer()
	if endpoint == "" && iface == nil && peer == nil {
		return nil
	}
	section := PreviewSection{Title: "WireGuard"}
	if endpoint != "" {
		section.Facts = append(section.Facts, PreviewFact{Label: "Endpoint", Value: endpoint})
	}
	if iface != nil {
		if addrs := strings.Join(iface.GetAddrs(), ", "); addrs != "" {
			section.Facts = append(section.Facts, PreviewFact{Label: "Адреса", Value: addrs})
		}
		if dns := strings.Join(iface.GetDns(), ", "); dns != "" {
			section.Facts = append(section.Facts, PreviewFact{Label: "DNS", Value: dns})
		}
		if iface.Mtu != nil && iface.GetMtu() > 0 {
			section.Facts = append(section.Facts, PreviewFact{Label: "MTU", Value: fmt.Sprintf("%d", iface.GetMtu())})
		}
	}
	if peer != nil && len(peer.GetAllowedIps()) > 0 {
		section.Facts = append(section.Facts, PreviewFact{
			Label: "Allowed IPs",
			Value: fmt.Sprintf("%d записей", len(peer.GetAllowedIps())),
		})
	}
	if len(section.Facts) == 0 {
		return nil
	}
	return &section
}

func buildAmneziaSection(awg *wingsvpb.AmneziaWG) *PreviewSection {
	if awg == nil {
		return nil
	}
	cfg := strings.TrimSpace(awg.GetAwgQuickConfig())
	if cfg == "" {
		return nil
	}
	lines := strings.Count(cfg, "\n") + 1
	return &PreviewSection{
		Title: "AmneziaWG",
		Facts: []PreviewFact{
			{Label: "AmneziaWG-quick", Value: fmt.Sprintf("%d строк", lines)},
		},
	}
}

func buildXposedSection(x *wingsvpb.Xposed) *PreviewSection {
	if x == nil {
		return nil
	}
	emptyTargets := len(x.GetTargetPackages()) == 0
	emptyHidden := len(x.GetHiddenVpnPackages()) == 0
	if x.Enabled == nil &&
		x.AllApps == nil &&
		x.NativeHookEnabled == nil &&
		x.InlineHooksEnabled == nil &&
		x.HideVpnApps == nil &&
		x.HideFromDumpsys == nil &&
		x.GetProcfsHookMode() == wingsvpb.XposedProcfsHookMode_XPOSED_PROCFS_HOOK_MODE_UNSPECIFIED &&
		x.GetIcmpSpoofingMode() == wingsvpb.XposedIcmpSpoofingMode_XPOSED_ICMP_SPOOFING_MODE_UNSPECIFIED &&
		emptyTargets &&
		emptyHidden {
		return nil
	}
	section := PreviewSection{Title: "Xposed модуль"}
	appendFact := func(label, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		section.Facts = append(section.Facts, PreviewFact{Label: label, Value: value})
	}
	if x.Enabled != nil {
		appendFact("Модуль", boolLabel(x.GetEnabled(), "Включён", "Выключен"))
	}
	if x.AllApps != nil {
		appendFact("Применять ко всем", boolLabel(x.GetAllApps(), "Да", "Нет"))
	}
	if x.NativeHookEnabled != nil {
		appendFact("Native hook", boolLabel(x.GetNativeHookEnabled(), "Включён", "Выключен"))
	}
	if x.InlineHooksEnabled != nil {
		appendFact("Inline hooks", boolLabel(x.GetInlineHooksEnabled(), "Включены", "Выключены"))
	}
	if x.HideVpnApps != nil {
		appendFact("Скрывать VPN-приложения", boolLabel(x.GetHideVpnApps(), "Да", "Нет"))
	}
	if x.HideFromDumpsys != nil {
		appendFact("Скрывать из dumpsys", boolLabel(x.GetHideFromDumpsys(), "Да", "Нет"))
	}
	if mode := xposedProcfsHookModeLabel(x.GetProcfsHookMode()); mode != "" {
		appendFact("ProcFS hook", mode)
	}
	if mode := xposedIcmpSpoofingModeLabel(x.GetIcmpSpoofingMode()); mode != "" {
		appendFact("ICMP spoofing", mode)
	}
	if !emptyTargets {
		appendFact("Целевых приложений", fmt.Sprintf("%d", len(x.GetTargetPackages())))
	}
	if !emptyHidden {
		appendFact("Скрываемых VPN-приложений", fmt.Sprintf("%d", len(x.GetHiddenVpnPackages())))
	}
	return &section
}

func xposedProcfsHookModeLabel(mode wingsvpb.XposedProcfsHookMode) string {
	switch mode {
	case wingsvpb.XposedProcfsHookMode_XPOSED_PROCFS_HOOK_MODE_DISABLED:
		return "Отключён"
	case wingsvpb.XposedProcfsHookMode_XPOSED_PROCFS_HOOK_MODE_FILTER:
		return "Filter"
	case wingsvpb.XposedProcfsHookMode_XPOSED_PROCFS_HOOK_MODE_NO_ACCESS:
		return "No access"
	case wingsvpb.XposedProcfsHookMode_XPOSED_PROCFS_HOOK_MODE_FILE_NOT_FOUND:
		return "File not found"
	default:
		return ""
	}
}

func xposedIcmpSpoofingModeLabel(mode wingsvpb.XposedIcmpSpoofingMode) string {
	switch mode {
	case wingsvpb.XposedIcmpSpoofingMode_XPOSED_ICMP_SPOOFING_MODE_DISABLED:
		return "Отключён"
	case wingsvpb.XposedIcmpSpoofingMode_XPOSED_ICMP_SPOOFING_MODE_PING_NOT_FOUND:
		return "Ping not found"
	case wingsvpb.XposedIcmpSpoofingMode_XPOSED_ICMP_SPOOFING_MODE_EMPTY_RESPONSE:
		return "Empty response"
	default:
		return ""
	}
}

func buildRootSettingsSection(root *wingsvpb.RootSettings) *PreviewSection {
	if root == nil {
		return nil
	}
	if root.Enabled == nil && root.KernelWireguard == nil && root.XrayTproxyMode == nil &&
		strings.TrimSpace(root.GetWgInterfaceName()) == "" {
		return nil
	}
	section := PreviewSection{Title: "Root режим"}
	appendFact := func(label, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		section.Facts = append(section.Facts, PreviewFact{Label: label, Value: value})
	}
	if root.Enabled != nil {
		appendFact("Root режим", boolLabel(root.GetEnabled(), "Включён", "Выключен"))
	}
	if root.KernelWireguard != nil {
		appendFact("Kernel WireGuard", boolLabel(root.GetKernelWireguard(), "Включён", "Выключен"))
	}
	if root.XrayTproxyMode != nil {
		appendFact("Xray TPROXY", boolLabel(root.GetXrayTproxyMode(), "Включён", "Выключен"))
	}
	if iface := strings.TrimSpace(root.GetWgInterfaceName()); iface != "" {
		appendFact("WG интерфейс", iface)
	}
	return &section
}

func buildGuardianSection(g *wingsvpb.Guardian) *PreviewSection {
	if g == nil {
		return nil
	}
	if g.GetWsUrl() == "" && g.GetClientId() == "" && len(g.GetClientToken()) == 0 {
		return nil
	}
	section := PreviewSection{
		Title: "Попечитель",
		Note: "После импорта владелец панели получит полный контроль: сможет менять любые настройки, " +
			"видеть логи и включать/выключать соединение. Импортируйте только если доверяете владельцу.",
	}
	if host := guardianHost(g.GetWsUrl()); host != "" {
		section.Facts = append(section.Facts, PreviewFact{Label: "Хост панели", Value: host})
	}
	if g.GetWsUrl() != "" {
		section.Facts = append(section.Facts, PreviewFact{Label: "WSS endpoint", Value: g.GetWsUrl()})
	}
	if id := strings.TrimSpace(g.GetClientId()); id != "" {
		section.Facts = append(section.Facts, PreviewFact{Label: "Client ID", Value: id})
	}
	if name := strings.TrimSpace(g.GetClientName()); name != "" {
		section.Facts = append(section.Facts, PreviewFact{Label: "Имя клиента", Value: name})
	}
	if len(g.GetClientToken()) > 0 {
		section.Facts = append(section.Facts, PreviewFact{
			Label: "Токен",
			Value: fmt.Sprintf("Встроен (%d байт). Не передавайте ссылку посторонним.", len(g.GetClientToken())),
		})
	}
	return &section
}

func guardianHost(wsURL string) string {
	if wsURL == "" {
		return ""
	}
	parsed, err := url.Parse(wsURL)
	if err != nil || parsed.Host == "" {
		return ""
	}
	return parsed.Host
}

func buildAppPreferencesSection(ap *wingsvpb.AppPreferences) *PreviewSection {
	if ap == nil {
		return nil
	}
	if ap.GetThemeMode() == wingsvpb.ThemeMode_THEME_MODE_UNSPECIFIED && ap.AutoStartOnBoot == nil {
		return nil
	}
	section := PreviewSection{Title: "Настройки приложения"}
	if mode := themeModeLabel(ap.GetThemeMode()); mode != "" {
		section.Facts = append(section.Facts, PreviewFact{Label: "Тема", Value: mode})
	}
	if ap.AutoStartOnBoot != nil {
		section.Facts = append(section.Facts, PreviewFact{
			Label: "Автозапуск при загрузке",
			Value: boolLabel(ap.GetAutoStartOnBoot(), "Да", "Нет"),
		})
	}
	return &section
}

func themeModeLabel(mode wingsvpb.ThemeMode) string {
	switch mode {
	case wingsvpb.ThemeMode_THEME_MODE_LIGHT:
		return "Светлая"
	case wingsvpb.ThemeMode_THEME_MODE_DARK:
		return "Тёмная"
	case wingsvpb.ThemeMode_THEME_MODE_SYSTEM:
		return "Системная"
	default:
		return ""
	}
}

func buildSubscriptionHwidSection(hwid *wingsvpb.SubscriptionHwid) *PreviewSection {
	if hwid == nil {
		return nil
	}
	if hwid.Enabled == nil && hwid.ManualEnabled == nil &&
		strings.TrimSpace(hwid.GetValue()) == "" &&
		strings.TrimSpace(hwid.GetDeviceOs()) == "" &&
		strings.TrimSpace(hwid.GetVerOs()) == "" &&
		strings.TrimSpace(hwid.GetDeviceModel()) == "" {
		return nil
	}
	section := PreviewSection{Title: "Subscription HWID"}
	appendFact := func(label, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		section.Facts = append(section.Facts, PreviewFact{Label: label, Value: value})
	}
	if hwid.Enabled != nil {
		appendFact("HWID-передача", boolLabel(hwid.GetEnabled(), "Включена", "Выключена"))
	}
	if hwid.ManualEnabled != nil {
		appendFact("Ручные значения", boolLabel(hwid.GetManualEnabled(), "Да", "Нет"))
	}
	appendFact("HWID", hwid.GetValue())
	appendFact("Device OS", hwid.GetDeviceOs())
	appendFact("OS version", hwid.GetVerOs())
	appendFact("Модель", hwid.GetDeviceModel())
	return &section
}

func buildSharingSection(sharing *wingsvpb.Sharing) *PreviewSection {
	if sharing == nil {
		return nil
	}
	emptyTypes := len(sharing.GetLastActiveTypes()) == 0
	if sharing.AutoStartOnBoot == nil &&
		emptyTypes &&
		strings.TrimSpace(sharing.GetUpstreamInterface()) == "" &&
		strings.TrimSpace(sharing.GetFallbackUpstreamInterface()) == "" &&
		sharing.GetMasqueradeMode() == wingsvpb.SharingMasqueradeMode_SHARING_MASQUERADE_MODE_UNSPECIFIED &&
		sharing.DisableIpv6 == nil &&
		sharing.DhcpWorkaround == nil &&
		sharing.GetWifiLock() == wingsvpb.SharingWifiLock_SHARING_WIFI_LOCK_UNSPECIFIED &&
		sharing.RepeaterSafeMode == nil &&
		sharing.TempHotspotUseSystem == nil &&
		sharing.GetIpMonitorMode() == wingsvpb.SharingIpMonitorMode_SHARING_IP_MONITOR_MODE_UNSPECIFIED {
		return nil
	}
	section := PreviewSection{Title: "Sharing / hotspot"}
	appendFact := func(label, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		section.Facts = append(section.Facts, PreviewFact{Label: label, Value: value})
	}
	if sharing.AutoStartOnBoot != nil {
		appendFact("Автозапуск при загрузке", boolLabel(sharing.GetAutoStartOnBoot(), "Да", "Нет"))
	}
	if !emptyTypes {
		appendFact("Транспорты", strings.Join(sharing.GetLastActiveTypes(), ", "))
	}
	appendFact("Upstream", sharing.GetUpstreamInterface())
	appendFact("Fallback upstream", sharing.GetFallbackUpstreamInterface())
	if mode := sharingMasqueradeModeLabel(sharing.GetMasqueradeMode()); mode != "" {
		appendFact("Masquerade", mode)
	}
	if sharing.DisableIpv6 != nil {
		appendFact("IPv6", boolLabel(sharing.GetDisableIpv6(), "Отключён", "Включён"))
	}
	if sharing.DhcpWorkaround != nil {
		appendFact("DHCP workaround", boolLabel(sharing.GetDhcpWorkaround(), "Да", "Нет"))
	}
	if mode := sharingWifiLockLabel(sharing.GetWifiLock()); mode != "" {
		appendFact("Wi-Fi lock", mode)
	}
	if sharing.RepeaterSafeMode != nil {
		appendFact("Repeater safe", boolLabel(sharing.GetRepeaterSafeMode(), "Да", "Нет"))
	}
	if sharing.TempHotspotUseSystem != nil {
		appendFact("Temp hotspot — system", boolLabel(sharing.GetTempHotspotUseSystem(), "Да", "Нет"))
	}
	if mode := sharingIpMonitorLabel(sharing.GetIpMonitorMode()); mode != "" {
		appendFact("IP monitor", mode)
	}
	return &section
}

func sharingMasqueradeModeLabel(mode wingsvpb.SharingMasqueradeMode) string {
	switch mode {
	case wingsvpb.SharingMasqueradeMode_SHARING_MASQUERADE_MODE_NONE:
		return "Отключён"
	case wingsvpb.SharingMasqueradeMode_SHARING_MASQUERADE_MODE_SIMPLE:
		return "Simple"
	case wingsvpb.SharingMasqueradeMode_SHARING_MASQUERADE_MODE_NETD:
		return "netd"
	default:
		return ""
	}
}

func sharingWifiLockLabel(mode wingsvpb.SharingWifiLock) string {
	switch mode {
	case wingsvpb.SharingWifiLock_SHARING_WIFI_LOCK_SYSTEM:
		return "System default"
	case wingsvpb.SharingWifiLock_SHARING_WIFI_LOCK_FULL:
		return "Full"
	case wingsvpb.SharingWifiLock_SHARING_WIFI_LOCK_HIGH_PERF:
		return "High perf"
	case wingsvpb.SharingWifiLock_SHARING_WIFI_LOCK_LOW_LATENCY:
		return "Low latency"
	default:
		return ""
	}
}

func sharingIpMonitorLabel(mode wingsvpb.SharingIpMonitorMode) string {
	switch mode {
	case wingsvpb.SharingIpMonitorMode_SHARING_IP_MONITOR_MODE_NETLINK:
		return "Netlink"
	case wingsvpb.SharingIpMonitorMode_SHARING_IP_MONITOR_MODE_NETLINK_ROOT:
		return "Netlink (root)"
	case wingsvpb.SharingIpMonitorMode_SHARING_IP_MONITOR_MODE_POLL:
		return "Poll"
	case wingsvpb.SharingIpMonitorMode_SHARING_IP_MONITOR_MODE_POLL_ROOT:
		return "Poll (root)"
	default:
		return ""
	}
}

func buildByeDpiSection(b *wingsvpb.ByeDpi) *PreviewSection {
	if b == nil {
		return nil
	}
	// Empty-message guard: if all key fields are unset, skip.
	if b.AutoStartWithXray == nil && b.UseCommandSettings == nil &&
		strings.TrimSpace(b.GetProxyIp()) == "" && b.ProxyPort == nil &&
		b.GetHostsMode() == wingsvpb.ByeDpiHostsMode_BYE_DPI_HOSTS_MODE_UNSPECIFIED &&
		b.GetDesyncMethod() == wingsvpb.ByeDpiDesyncMethod_BYE_DPI_DESYNC_METHOD_UNSPECIFIED &&
		strings.TrimSpace(b.GetCmdArgs()) == "" {
		return nil
	}
	section := PreviewSection{Title: "ByeDPI"}
	appendFact := func(label, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		section.Facts = append(section.Facts, PreviewFact{Label: label, Value: value})
	}
	if b.AutoStartWithXray != nil {
		appendFact("Запуск с Xray", boolLabel(b.GetAutoStartWithXray(), "Да", "Нет"))
	}
	if b.UseCommandSettings != nil {
		appendFact("Режим", boolLabel(b.GetUseCommandSettings(), "Командная строка", "UI"))
	}
	if proxyIp := strings.TrimSpace(b.GetProxyIp()); proxyIp != "" {
		host := proxyIp
		if b.ProxyPort != nil {
			host = fmt.Sprintf("%s:%d", proxyIp, b.GetProxyPort())
		}
		appendFact("Listen", host)
	} else if b.ProxyPort != nil {
		appendFact("Port", fmt.Sprintf("%d", b.GetProxyPort()))
	}
	if mode := byeDpiHostsModeLabel(b.GetHostsMode()); mode != "" {
		appendFact("Hosts", mode)
	}
	if mode := byeDpiDesyncMethodLabel(b.GetDesyncMethod()); mode != "" {
		appendFact("Desync", mode)
	}
	if b.UseCommandSettings != nil && b.GetUseCommandSettings() {
		if cmd := strings.TrimSpace(b.GetCmdArgs()); cmd != "" {
			args := strings.Fields(cmd)
			appendFact("Cmd args", fmt.Sprintf("%d токенов", len(args)))
		}
	}
	return &section
}

func byeDpiHostsModeLabel(mode wingsvpb.ByeDpiHostsMode) string {
	switch mode {
	case wingsvpb.ByeDpiHostsMode_BYE_DPI_HOSTS_MODE_DISABLE:
		return "Отключены"
	case wingsvpb.ByeDpiHostsMode_BYE_DPI_HOSTS_MODE_BLACKLIST:
		return "Blacklist"
	case wingsvpb.ByeDpiHostsMode_BYE_DPI_HOSTS_MODE_WHITELIST:
		return "Whitelist"
	default:
		return ""
	}
}

func byeDpiDesyncMethodLabel(mode wingsvpb.ByeDpiDesyncMethod) string {
	switch mode {
	case wingsvpb.ByeDpiDesyncMethod_BYE_DPI_DESYNC_METHOD_NONE:
		return "None"
	case wingsvpb.ByeDpiDesyncMethod_BYE_DPI_DESYNC_METHOD_SPLIT:
		return "Split"
	case wingsvpb.ByeDpiDesyncMethod_BYE_DPI_DESYNC_METHOD_DISORDER:
		return "Disorder"
	case wingsvpb.ByeDpiDesyncMethod_BYE_DPI_DESYNC_METHOD_FAKE:
		return "Fake"
	case wingsvpb.ByeDpiDesyncMethod_BYE_DPI_DESYNC_METHOD_OOB:
		return "OOB"
	case wingsvpb.ByeDpiDesyncMethod_BYE_DPI_DESYNC_METHOD_DISOOB:
		return "DisOOB"
	default:
		return ""
	}
}

func buildWbStreamSection(wb *wingsvpb.WbStream) *PreviewSection {
	if wb == nil {
		return nil
	}
	roomID := strings.TrimSpace(wb.GetRoomId())
	displayName := strings.TrimSpace(wb.GetDisplayName())
	if roomID == "" && displayName == "" && !wb.GetExchangeViaVkTurn() && !wb.GetE2EEnabled() {
		return nil
	}
	section := PreviewSection{Title: "WB Stream"}
	if roomID != "" {
		section.Facts = append(section.Facts, PreviewFact{Label: "Room ID", Value: roomID})
	}
	if displayName != "" {
		section.Facts = append(section.Facts, PreviewFact{Label: "Display name", Value: displayName})
	}
	section.Facts = append(section.Facts, PreviewFact{
		Label: "Обмен room id через VK TURN",
		Value: boolLabel(wb.GetExchangeViaVkTurn(), "Да", "Нет"),
	})
	section.Facts = append(section.Facts, PreviewFact{
		Label: "E2E шифрование",
		Value: boolLabel(wb.GetE2EEnabled(), "Включено", "Выключено"),
	})
	return &section
}

func buildAppRoutingSection(app *wingsvpb.AppRouting) *PreviewSection {
	if app == nil {
		return nil
	}
	if app.Bypass == nil && len(app.GetPackages()) == 0 {
		return nil
	}
	section := PreviewSection{Title: "Per-app routing"}
	if app.Bypass != nil {
		section.Facts = append(section.Facts, PreviewFact{
			Label: "Режим",
			Value: boolLabel(app.GetBypass(), "Bypass", "Только эти приложения"),
		})
	}
	pkgs := app.GetPackages()
	if len(pkgs) > 0 {
		section.Facts = append(section.Facts, PreviewFact{
			Label: "Пакетов",
			Value: fmt.Sprintf("%d", len(pkgs)),
		})
		preview := pkgs
		const maxPreview = 8
		truncated := false
		if len(preview) > maxPreview {
			preview = preview[:maxPreview]
			truncated = true
		}
		joined := strings.Join(preview, ", ")
		if truncated {
			joined = fmt.Sprintf("%s … +%d", joined, len(pkgs)-maxPreview)
		}
		section.Facts = append(section.Facts, PreviewFact{
			Label: "Список",
			Value: joined,
		})
	}
	return &section
}

func normalizeBase64(value string) string {
	value = strings.TrimSpace(value)
	return strings.TrimRight(value, "=")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func endpointLabel(endpoint *wingsvpb.Endpoint) string {
	if endpoint == nil {
		return ""
	}
	host := strings.TrimSpace(endpoint.GetHost())
	port := endpoint.GetPort()
	if host == "" && port == 0 {
		return ""
	}
	if port == 0 {
		return host
	}
	return fmt.Sprintf("%s:%d", host, port)
}

func boolLabel(value bool, yes, no string) string {
	if value {
		return yes
	}
	return no
}

func backendLabel(backend wingsvpb.BackendType) string {
	switch backend {
	case wingsvpb.BackendType_BACKEND_TYPE_VK_TURN_WIREGUARD:
		return "VK TURN + WireGuard"
	case wingsvpb.BackendType_BACKEND_TYPE_XRAY:
		return "Xray"
	case wingsvpb.BackendType_BACKEND_TYPE_AMNEZIAWG:
		return "AmneziaWG"
	case wingsvpb.BackendType_BACKEND_TYPE_WIREGUARD:
		return "WireGuard"
	case wingsvpb.BackendType_BACKEND_TYPE_AMNEZIAWG_PLAIN:
		return "AmneziaWG Plain"
	case wingsvpb.BackendType_BACKEND_TYPE_WB_STREAM:
		return "WB Stream"
	default:
		return "WINGS V"
	}
}

func configTypeLabel(configType wingsvpb.ConfigType) string {
	switch configType {
	case wingsvpb.ConfigType_CONFIG_TYPE_VK:
		return "VK TURN"
	case wingsvpb.ConfigType_CONFIG_TYPE_XRAY:
		return "Xray"
	case wingsvpb.ConfigType_CONFIG_TYPE_AMNEZIAWG:
		return "AmneziaWG"
	case wingsvpb.ConfigType_CONFIG_TYPE_ALL:
		return "Все настройки"
	case wingsvpb.ConfigType_CONFIG_TYPE_APP_ROUTING:
		return "Bypass"
	case wingsvpb.ConfigType_CONFIG_TYPE_XRAY_ROUTING:
		return "Xray Routing"
	case wingsvpb.ConfigType_CONFIG_TYPE_WB_STREAM:
		return "WB Stream"
	case wingsvpb.ConfigType_CONFIG_TYPE_XPOSED:
		return "Xposed"
	case wingsvpb.ConfigType_CONFIG_TYPE_GUARDIAN:
		return "Попечитель"
	default:
		return "WINGS V"
	}
}

func turnSessionModeLabel(mode wingsvpb.TurnSessionMode) string {
	switch mode {
	case wingsvpb.TurnSessionMode_TURN_SESSION_MODE_AUTO:
		return "Auto"
	case wingsvpb.TurnSessionMode_TURN_SESSION_MODE_MAINLINE:
		return "Mainline"
	case wingsvpb.TurnSessionMode_TURN_SESSION_MODE_MUX:
		return "MU"
	default:
		return ""
	}
}

func proxyRuntimeModeLabel(mode wingsvpb.ProxyRuntimeMode) string {
	switch mode {
	case wingsvpb.ProxyRuntimeMode_PROXY_RUNTIME_MODE_VPN:
		return "VPN"
	case wingsvpb.ProxyRuntimeMode_PROXY_RUNTIME_MODE_PROXY:
		return "Proxy-only"
	default:
		return ""
	}
}

func xrayTransportModeLabel(mode wingsvpb.XrayTransportMode) string {
	switch mode {
	case wingsvpb.XrayTransportMode_XRAY_TRANSPORT_MODE_DIRECT:
		return "Direct"
	case wingsvpb.XrayTransportMode_XRAY_TRANSPORT_MODE_VK_TURN_TCP:
		return "VK TURN TCP"
	default:
		return ""
	}
}
