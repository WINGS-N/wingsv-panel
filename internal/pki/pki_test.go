package pki

import (
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
	"time"
)

func mustLeaf(t *testing.T, ca *CA, hosts []string) (certPEM, keyPEM []byte) {
	t.Helper()
	certPEM, keyPEM, err := ca.IssueLeaf(hosts, time.Hour)
	if err != nil {
		t.Fatalf("IssueLeaf: %v", err)
	}
	return certPEM, keyPEM
}

func parseLeaf(t *testing.T, certPEM []byte) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("no PEM block")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse leaf: %v", err)
	}
	return cert
}

func verifyChain(ca *CA, leaf *x509.Certificate, dnsName string) error {
	roots := x509.NewCertPool()
	roots.AddCert(ca.Certificate())
	_, err := leaf.Verify(x509.VerifyOptions{
		Roots:     roots,
		DNSName:   dnsName,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	})
	return err
}

func TestPinStableAcrossLeafReissue(t *testing.T) {
	ca, err := GenerateCA("wingsv-panel test", 10*365*24*time.Hour)
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	before := ca.Pin()
	mustLeaf(t, ca, []string{"panel.example"})
	mustLeaf(t, ca, []string{"10.0.0.5"})
	after := ca.Pin()
	if before != after {
		t.Fatal("pin changed after issuing leaves; must hash the CA SPKI only")
	}
	if len(before) != PinLen || PinLen != 32 {
		t.Fatalf("pin length = %d, want 32", len(before))
	}
	// "sha256/" + base64(32 bytes, padded) = 7 + 44 = 51 chars.
	if got := ca.PinBase64(); len(got) != 51 || !strings.HasPrefix(got, "sha256/") {
		t.Fatalf("PinBase64 = %q (len %d), want sha256/ + 44 base64 chars", got, len(got))
	}
}

func TestLeafVerifiesAgainstCA(t *testing.T) {
	ca, err := GenerateCA("wingsv-panel test", time.Hour)
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	certPEM, _ := mustLeaf(t, ca, []string{"panel.example", "10.0.0.5"})
	leaf := parseLeaf(t, certPEM)
	if err := verifyChain(ca, leaf, "panel.example"); err != nil {
		t.Fatalf("verify by DNS SAN: %v", err)
	}
	if len(leaf.IPAddresses) != 1 || leaf.IPAddresses[0].String() != "10.0.0.5" {
		t.Fatalf("IP SAN not carried: %v", leaf.IPAddresses)
	}
}

func TestUnrelatedCertRejected(t *testing.T) {
	ca, err := GenerateCA("wingsv-panel test", time.Hour)
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	other, err := GenerateCA("attacker", time.Hour)
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	certPEM, _ := mustLeaf(t, other, []string{"panel.example"})
	leaf := parseLeaf(t, certPEM)
	if err := verifyChain(ca, leaf, "panel.example"); err == nil {
		t.Fatal("leaf from a different CA verified against our CA; want rejection")
	}
	if ca.Pin() == other.Pin() {
		t.Fatal("two independent CAs share a pin")
	}
}

func TestLoadCARoundTrip(t *testing.T) {
	ca, err := GenerateCA("wingsv-panel test", time.Hour)
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	loaded, err := LoadCA(ca.CertPEM(), ca.KeyPEM())
	if err != nil {
		t.Fatalf("LoadCA: %v", err)
	}
	if loaded.Pin() != ca.Pin() {
		t.Fatal("pin differs after load")
	}
	// A leaf issued by the reloaded CA must still chain to the original cert.
	certPEM, _ := mustLeaf(t, loaded, []string{"panel.example"})
	leaf := parseLeaf(t, certPEM)
	if err := verifyChain(ca, leaf, "panel.example"); err != nil {
		t.Fatalf("leaf from reloaded CA does not chain to original: %v", err)
	}
}

func TestPinFromCertPEMMatchesCAPin(t *testing.T) {
	ca, err := GenerateCA("wingsv-panel test", time.Hour)
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	got, err := PinFromCertPEM(ca.CertPEM())
	if err != nil {
		t.Fatalf("PinFromCertPEM: %v", err)
	}
	if got != ca.Pin() {
		t.Fatal("PinFromCertPEM(CA cert) != CA.Pin()")
	}
}
