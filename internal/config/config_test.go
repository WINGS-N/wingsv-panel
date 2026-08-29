package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigFilePrecedence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	body := "" +
		"# panel config\n" +
		"PUBLIC_BASE_URL = \"https://from-file.example\"\n" +
		"RELAY_TOKEN = 'token-from-file'\n" +
		"SESSION_SECURE = false  # inline comment\n" +
		"PROVISIONING_LISTEN = \":9091\"\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WINGS_PANEL_CONFIG", path)
	// Env overrides the file for this one key.
	t.Setenv("RELAY_TOKEN", "token-from-env")

	cfg := Load()

	if cfg.PublicBaseURL != "https://from-file.example" {
		t.Errorf("PublicBaseURL from file = %q", cfg.PublicBaseURL)
	}
	if cfg.RelayToken != "token-from-env" {
		t.Errorf("RelayToken should be env-overridden, got %q", cfg.RelayToken)
	}
	if cfg.SessionSecure {
		t.Errorf("SessionSecure should be false from file")
	}
	if cfg.ProvisioningListen != ":9091" {
		t.Errorf("ProvisioningListen from file = %q", cfg.ProvisioningListen)
	}
	// A key absent from both env and file keeps its default.
	if cfg.DBKind != "sqlite" {
		t.Errorf("DBKind default = %q", cfg.DBKind)
	}
}
