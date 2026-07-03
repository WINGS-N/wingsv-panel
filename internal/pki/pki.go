// Package pki implements the control-plane TLS trust for wingsv-panel: a
// per-deployment self-signed CA whose SubjectPublicKeyInfo hash (the SPKI pin)
// is embedded in wingsv:// enrollment links so the WINGS V app can verify the
// panel's Guardian-WSS / gRPC / web endpoints without a domain or an external
// CA. Leaf certificates rotate freely under the pinned CA. This trust applies
// ONLY to control-plane channels - never to the VK TURN upper DTLS relay.
//
// The pin is SHA-256 over the CA certificate's SubjectPublicKeyInfo (32 bytes),
// which is stable across leaf reissues because it hashes the CA public key, not
// the leaf.
package pki

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"time"
)

// PinLen is the length in bytes of an SPKI pin (SHA-256 digest).
const PinLen = sha256.Size

// CA is a self-signed certificate authority plus its private key, used to issue
// server leaf certificates for the panel's control-plane listeners.
type CA struct {
	cert    *x509.Certificate
	key     *ecdsa.PrivateKey
	certPEM []byte
	keyPEM  []byte
}

// GenerateCA creates a new self-signed CA (ECDSA P-256) valid for validFor from
// now. commonName labels the CA subject; it is not verified by clients (they
// pin the SPKI), so any stable descriptive name is fine.
func GenerateCA(commonName string, validFor time.Duration) (*CA, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("pki: generate ca key: %w", err)
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(validFor),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, fmt.Errorf("pki: create ca cert: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("pki: parse ca cert: %w", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM, err := marshalECKeyPEM(key)
	if err != nil {
		return nil, err
	}
	return &CA{cert: cert, key: key, certPEM: certPEM, keyPEM: keyPEM}, nil
}

// LoadCA reconstructs a CA from its PEM-encoded certificate and private key,
// as produced by CertPEM and KeyPEM.
func LoadCA(certPEM, keyPEM []byte) (*CA, error) {
	certBlock, _ := pem.Decode(certPEM)
	if certBlock == nil || certBlock.Type != "CERTIFICATE" {
		return nil, errors.New("pki: invalid ca certificate PEM")
	}
	cert, err := x509.ParseCertificate(certBlock.Bytes)
	if err != nil {
		return nil, fmt.Errorf("pki: parse ca cert: %w", err)
	}
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, errors.New("pki: invalid ca key PEM")
	}
	key, err := parseECKey(keyBlock.Bytes)
	if err != nil {
		return nil, err
	}
	return &CA{cert: cert, key: key, certPEM: certPEM, keyPEM: keyPEM}, nil
}

// CertPEM returns the CA certificate in PEM form (safe to persist / distribute).
func (c *CA) CertPEM() []byte { return c.certPEM }

// KeyPEM returns the CA private key in PEM form. Treat as a secret: anyone with
// it can mint leaves the pinned app will trust. Persist with 0600 perms.
func (c *CA) KeyPEM() []byte { return c.keyPEM }

// Certificate returns the parsed CA certificate.
func (c *CA) Certificate() *x509.Certificate { return c.cert }

// Pin returns the SPKI pin (SHA-256 of the CA's SubjectPublicKeyInfo). These are
// the 32 raw bytes to place in the Guardian.server_ca_pins link field.
func (c *CA) Pin() [PinLen]byte {
	return sha256.Sum256(c.cert.RawSubjectPublicKeyInfo)
}

// PinBase64 returns the HPKP-style "sha256/<base64>" form of the pin, for logs,
// CLI output, and human display.
func (c *CA) PinBase64() string {
	pin := c.Pin()
	return "sha256/" + base64.StdEncoding.EncodeToString(pin[:])
}

// IssueLeaf signs a server leaf certificate for the given hosts, valid for
// validFor from now. Each host that parses as an IP goes into IPAddresses; the
// rest go into DNSNames. The returned key is a fresh ECDSA P-256 key.
func (c *CA) IssueLeaf(hosts []string, validFor time.Duration) (certPEM, keyPEM []byte, err error) {
	if len(hosts) == 0 {
		return nil, nil, errors.New("pki: issue leaf requires at least one host")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("pki: generate leaf key: %w", err)
	}
	serial, err := randomSerial()
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: hosts[0]},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(validFor),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, c.cert, &key.PublicKey, c.key)
	if err != nil {
		return nil, nil, fmt.Errorf("pki: create leaf cert: %w", err)
	}
	certPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM, err = marshalECKeyPEM(key)
	if err != nil {
		return nil, nil, err
	}
	return certPEM, keyPEM, nil
}

// PinFromCertPEM computes the SPKI pin of a certificate given its PEM. Useful on
// the verifying side to check a served leaf's issuer against a stored pin.
func PinFromCertPEM(certPEM []byte) ([PinLen]byte, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return [PinLen]byte{}, errors.New("pki: invalid certificate PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return [PinLen]byte{}, fmt.Errorf("pki: parse cert: %w", err)
	}
	return sha256.Sum256(cert.RawSubjectPublicKeyInfo), nil
}

func randomSerial() (*big.Int, error) {
	// 128-bit random serial, per CA/Browser Forum guidance.
	limit := new(big.Int).Lsh(big.NewInt(1), 128)
	serial, err := rand.Int(rand.Reader, limit)
	if err != nil {
		return nil, fmt.Errorf("pki: serial: %w", err)
	}
	return serial, nil
}

func marshalECKeyPEM(key *ecdsa.PrivateKey) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("pki: marshal key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), nil
}

func parseECKey(der []byte) (*ecdsa.PrivateKey, error) {
	if k, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		ec, ok := k.(*ecdsa.PrivateKey)
		if !ok {
			return nil, errors.New("pki: ca key is not ECDSA")
		}
		return ec, nil
	}
	ec, err := x509.ParseECPrivateKey(der)
	if err != nil {
		return nil, fmt.Errorf("pki: parse ca key: %w", err)
	}
	return ec, nil
}
