package admin

import (
	"bytes"
	"testing"

	"v.wingsnet.org/internal/config"
	"v.wingsnet.org/internal/pki"
)

func TestCaPinsLoadedFromCADir(t *testing.T) {
	dir := t.TempDir()
	ca, _, err := pki.LoadOrCreateCA(dir)
	if err != nil {
		t.Fatalf("LoadOrCreateCA: %v", err)
	}
	h := New(config.Config{CADir: dir}, nil, nil, nil)
	if len(h.caPins) != 1 {
		t.Fatalf("caPins = %d, want 1", len(h.caPins))
	}
	pin := ca.Pin()
	if !bytes.Equal(h.caPins[0], pin[:]) {
		t.Fatal("emitted pin does not match the CA SPKI pin")
	}
}

func TestNoCaPinsWithoutCA(t *testing.T) {
	h := New(config.Config{CADir: t.TempDir()}, nil, nil, nil)
	if h.caPins != nil {
		t.Fatalf("caPins should be nil without a CA, got %v", h.caPins)
	}
}
