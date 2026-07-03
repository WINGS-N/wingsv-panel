package config

import (
	"os"
	"strconv"
	"strings"
)

type Config struct {
	ListenAddr         string
	PublicBaseURL      string
	AssetLinksJSON     string
	GitHubRepo         string
	ReleaseAssetSuffix string
	StaticDir          string
	DBPath             string
	DBKind             string
	DBDSN              string
	CADir              string
	ProvisioningListen string
	RelayToken         string
	// VkTurnEndpoint is the VK TURN relay address the app dials to reach the
	// panel-local vk-turn-proxy node. It is embedded in managed VK-TURN-profile
	// enrollment links so the app can self-provision its wg config (M3).
	VkTurnEndpoint         string
	BootstrapAdminUsername string
	BootstrapAdminPassword string
	SessionSecure          bool
}

func Load() Config {
	return Config{
		ListenAddr:         getEnv("LISTEN_ADDR", ":8080"),
		PublicBaseURL:      strings.TrimRight(getEnv("PUBLIC_BASE_URL", "https://v.wingsnet.org"), "/"),
		AssetLinksJSON:     getEnv("ASSET_LINKS_JSON", ""),
		GitHubRepo:         getEnv("GITHUB_REPO", "WINGS-N/WINGSV"),
		ReleaseAssetSuffix: getEnv("RELEASE_ASSET_SUFFIX", ".apk"),
		// Default empty so the embedded SPA bundle is served. Set STATIC_DIR
		// explicitly to swap the frontend without rebuilding the binary.
		StaticDir:              getEnv("STATIC_DIR", ""),
		DBPath:                 getEnv("DB_PATH", "./v-wingsnet.db"),
		DBKind:                 getEnv("DB_KIND", "sqlite"),
		DBDSN:                  getEnv("DB_DSN", ""),
		CADir:                  getEnv("CA_DIR", "./certs"),
		ProvisioningListen:     getEnv("PROVISIONING_LISTEN", ""),
		RelayToken:             getEnv("RELAY_TOKEN", ""),
		VkTurnEndpoint:         getEnv("VK_TURN_ENDPOINT", ""),
		BootstrapAdminUsername: getEnv("BOOTSTRAP_ADMIN_USERNAME", "admin"),
		BootstrapAdminPassword: getEnv("BOOTSTRAP_ADMIN_PASSWORD", "admin"),
		SessionSecure:          parseBoolEnv("SESSION_SECURE", true),
	}
}

func parseBoolEnv(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
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
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func ParseIntEnv(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
