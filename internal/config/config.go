package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Config struct {
	ListenAddr         string
	PublicBaseURL      string
	AssetLinksJSON     string
	GitHubRepo         string
	ReleaseAssetSuffix string
	// APKCacheDir holds the downloaded release asset. Ephemeral by design: it is
	// a cache, and losing it only costs one re-download.
	APKCacheDir string
	StaticDir   string
	DBPath      string
	DBKind      string
	DBDSN       string
	CADir       string
	// TLSCert / TLSKey point at a PEM certificate + key so the panel serves HTTPS
	// itself (Let's Encrypt via acme.sh, or bring-your-own). Empty = no direct TLS.
	TLSCert string
	TLSKey  string
	// TLSSelfSigned makes the panel serve a leaf issued by the deployment CA
	// (CA_DIR) for the PublicBaseURL host, so a no-domain install gets HTTPS whose
	// SPKI pin the app already embeds in enrollment links.
	TLSSelfSigned      bool
	ProvisioningListen string
	RelayToken         string
	// FederationHead is the wingsvpn-federation head's panel-facing gRPC address.
	// Empty keeps every federation surface off, which is the default.
	FederationHead string
	// Вход через собственный сервис учёток. Issuer прибит гвоздями: личность,
	// выданную где-то ещё, никто не оплачивал, а инвайт-дерево держится ровно на
	// том, что она чего-то стоит. Пусто - панель живёт на своём пароле, и админ,
	// поднявший её у себя, про OIDC не узнает
	OIDCIssuer       string
	OIDCClientID     string
	OIDCClientSecret string
	// AccountName - как звать эту дверь на экране входа
	AccountName string

	// FederationSecret keys that link. It is deliberately not the fleet secret
	// every donated node holds: that would hand a donor the operator's view.
	FederationSecret string

	BootstrapAdminUsername string
	BootstrapAdminPassword string
	SessionSecure          bool
}

func Load() Config {
	fileVals = loadConfigFile()
	return Config{
		ListenAddr:         getEnv("LISTEN_ADDR", ":8080"),
		PublicBaseURL:      strings.TrimRight(getEnv("PUBLIC_BASE_URL", "https://v.wingsnet.org"), "/"),
		AssetLinksJSON:     getEnv("ASSET_LINKS_JSON", ""),
		GitHubRepo:         getEnv("GITHUB_REPO", "WINGS-N/WINGSV"),
		ReleaseAssetSuffix: getEnv("RELEASE_ASSET_SUFFIX", ".apk"),
		APKCacheDir:        getEnv("APK_CACHE_DIR", filepath.Join(os.TempDir(), "wingsv-apk")),
		// Default empty so the embedded SPA bundle is served. Set STATIC_DIR
		// explicitly to swap the frontend without rebuilding the binary.
		StaticDir:          getEnv("STATIC_DIR", ""),
		DBPath:             getEnv("DB_PATH", "./v-wingsnet.db"),
		DBKind:             getEnv("DB_KIND", "sqlite"),
		DBDSN:              getEnv("DB_DSN", ""),
		CADir:              getEnv("CA_DIR", "./certs"),
		TLSCert:            getEnv("TLS_CERT", ""),
		TLSKey:             getEnv("TLS_KEY", ""),
		TLSSelfSigned:      parseBoolEnv("TLS_SELF_SIGNED", false),
		ProvisioningListen: getEnv("PROVISIONING_LISTEN", ""),
		RelayToken:         getEnv("RELAY_TOKEN", ""),
		// Слеш на конце НЕ срезаем: go-oidc сверяет issuer из discovery со
		// строкой байт в байт, а MAS отдаёт его со слешем
		OIDCIssuer:             getEnv("OIDC_ISSUER", ""),
		AccountName:            getEnv("ACCOUNT_NAME", "WINGS Account"),
		OIDCClientID:           getEnv("OIDC_CLIENT_ID", ""),
		OIDCClientSecret:       getEnv("OIDC_CLIENT_SECRET", ""),
		FederationHead:         getEnv("FEDERATION_HEAD", ""),
		FederationSecret:       getEnv("FEDERATION_SECRET", ""),
		BootstrapAdminUsername: getEnv("BOOTSTRAP_ADMIN_USERNAME", "admin"),
		BootstrapAdminPassword: getEnv("BOOTSTRAP_ADMIN_PASSWORD", "admin"),
		SessionSecure:          parseBoolEnv("SESSION_SECURE", true),
	}
}

// fileVals holds values read from the on-disk config file (a flat KEY = "value"
// TOML subset), refreshed by Load. Precedence is env > file > default: an env var
// always wins, the file fills gaps, then the hard-coded fallback.
var fileVals = map[string]string{}

// loadConfigFile reads /etc/wings/panel/config.toml (override with the
// WINGS_PANEL_CONFIG env var). It parses only flat KEY = value lines with the same
// KEY names as the env vars; nested tables and arrays are not supported - this is
// a standalone-binary convenience, not a full TOML config. A missing file yields
// an empty map so env/defaults still apply.
func loadConfigFile() map[string]string {
	path := strings.TrimSpace(os.Getenv("WINGS_PANEL_CONFIG"))
	if path == "" {
		path = "/etc/wings/panel/config.toml"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]string{}
	}
	out := map[string]string{}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		if key == "" {
			continue
		}
		out[key] = unquoteTOMLValue(strings.TrimSpace(line[eq+1:]))
	}
	return out
}

// unquoteTOMLValue strips surrounding quotes from a quoted value, or drops a
// trailing inline comment from a bare value.
func unquoteTOMLValue(v string) string {
	if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') {
		if end := strings.IndexByte(v[1:], v[0]); end >= 0 {
			return v[1 : 1+end]
		}
	}
	if hash := strings.IndexByte(v, '#'); hash >= 0 {
		v = v[:hash]
	}
	return strings.TrimSpace(v)
}

// lookup returns a setting from the environment (highest priority) or the config
// file, or "" when neither has it.
func lookup(key string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	if value, ok := fileVals[key]; ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func parseBoolEnv(key string, fallback bool) bool {
	value := lookup(key)
	if value == "" {
		return fallback
	}
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return fallback
}

func getEnv(key string, fallback string) string {
	if value := lookup(key); value != "" {
		return value
	}
	return fallback
}

func ParseIntEnv(key string, fallback int) int {
	value := lookup(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
