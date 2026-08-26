package pki

import (
	"crypto/tls"
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
