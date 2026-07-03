package pki

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

const (
	caCertFile = "ca.crt"
	caKeyFile  = "ca.key"

	// DefaultCAValidity is the lifetime of a freshly generated panel CA. Leaf
	// certs rotate under it, so it is deliberately long.
	DefaultCAValidity = 10 * 365 * 24 * time.Hour
)

// LoadCADir loads the CA persisted in dir by SaveCADir.
func LoadCADir(dir string) (*CA, error) {
	certPEM, err := os.ReadFile(filepath.Join(dir, caCertFile))
	if err != nil {
		return nil, err
	}
	keyPEM, err := os.ReadFile(filepath.Join(dir, caKeyFile))
	if err != nil {
		return nil, err
	}
	return LoadCA(certPEM, keyPEM)
}

// SaveCADir writes the CA certificate (world-readable) and key (0600) into dir.
func SaveCADir(dir string, ca *CA) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, caCertFile), ca.CertPEM(), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, caKeyFile), ca.KeyPEM(), 0o600)
}

// LoadOrCreateCA returns the CA persisted in dir, generating and saving a fresh
// one on first use. The created flag reports whether a new CA was generated.
func LoadOrCreateCA(dir string) (ca *CA, created bool, err error) {
	ca, err = LoadCADir(dir)
	if err == nil {
		return ca, false, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, false, err
	}
	ca, err = GenerateCA("WINGS V Panel CA", DefaultCAValidity)
	if err != nil {
		return nil, false, err
	}
	if err := SaveCADir(dir, ca); err != nil {
		return nil, false, err
	}
	return ca, true, nil
}
