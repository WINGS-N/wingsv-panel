package pki

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"time"
)

// ServerTLSCertificate issues a leaf for hosts and returns a tls.Certificate
// that presents the leaf followed by the CA, so a client pinning the CA's SPKI
// can find and verify the CA in the presented chain.
func (c *CA) ServerTLSCertificate(hosts []string, validFor time.Duration) (tls.Certificate, error) {
	certPEM, keyPEM, err := c.IssueLeaf(hosts, validFor)
	if err != nil {
		return tls.Certificate{}, err
	}
	chainPEM := append(append([]byte{}, certPEM...), c.certPEM...)
	return tls.X509KeyPair(chainPEM, keyPEM)
}

// PinnedClientTLSConfig returns a tls.Config that skips the default hostname and
// chain validation and instead requires that some certificate the server
// presents has a SubjectPublicKeyInfo hash matching one of pins. This is the
// control-plane trust for a panel / node that uses a self-signed CA rather than
// a publicly trusted certificate.
func PinnedClientTLSConfig(pins ...[PinLen]byte) *tls.Config {
	return &tls.Config{
		InsecureSkipVerify: true,
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			for _, raw := range rawCerts {
				cert, err := x509.ParseCertificate(raw)
				if err != nil {
					continue
				}
				got := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
				for _, pin := range pins {
					if got == pin {
						return nil
					}
				}
			}
			return errors.New("pki: no presented certificate matches a pinned CA")
		},
	}
}
