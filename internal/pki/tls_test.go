package pki

import (
	"crypto/tls"
	"net"
	"testing"
	"time"
)

func tlsServer(t *testing.T, ca *CA) net.Listener {
	t.Helper()
	cert, err := ca.ServerTLSCertificate([]string{"localhost"}, time.Hour)
	if err != nil {
		t.Fatalf("ServerTLSCertificate: %v", err)
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{cert}})
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				_ = conn.(*tls.Conn).Handshake()
				_ = conn.Close()
			}()
		}
	}()
	t.Cleanup(func() { _ = ln.Close() })
	return ln
}

func TestPinnedClientAcceptsMatchingCA(t *testing.T) {
	ca, err := GenerateCA("panel", time.Hour)
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	ln := tlsServer(t, ca)

	conn, err := tls.Dial("tcp", ln.Addr().String(), PinnedClientTLSConfig(ca.Pin()))
	if err != nil {
		t.Fatalf("pinned dial should succeed: %v", err)
	}
	_ = conn.Close()
}

func TestPinnedClientRejectsUnrelatedCA(t *testing.T) {
	ca, err := GenerateCA("panel", time.Hour)
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	other, err := GenerateCA("attacker", time.Hour)
	if err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}
	ln := tlsServer(t, ca)

	conn, err := tls.Dial("tcp", ln.Addr().String(), PinnedClientTLSConfig(other.Pin()))
	if err == nil {
		_ = conn.Close()
		t.Fatal("dial with a non-matching pin should fail")
	}
}
