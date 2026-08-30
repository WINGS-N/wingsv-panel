package tokenaead

import (
	"testing"
	"time"
)

func TestUnknownPeerGetsTheNewDerivation(t *testing.T) {
	p := NewPreference()
	if got := p.Next("node-1"); got != SHA512 {
		t.Errorf("Next = %v, want sha512 for a peer never seen", got)
	}
}

func TestAFailureFlipsToTheOtherDerivation(t *testing.T) {
	p := NewPreference()
	p.Failed("node-1", SHA512)
	if got := p.Next("node-1"); got != Legacy256 {
		t.Errorf("Next = %v after a sha512 failure, want sha256", got)
	}
	p.Failed("node-1", Legacy256)
	if got := p.Next("node-1"); got != SHA512 {
		t.Errorf("Next = %v after a sha256 failure, want sha512", got)
	}
}

func TestSuccessPins(t *testing.T) {
	p := NewPreference()
	p.Succeeded("node-1", Legacy256)
	for i := 0; i < 3; i++ {
		if got := p.Next("node-1"); got != Legacy256 {
			t.Fatalf("Next = %v, want the pinned sha256", got)
		}
	}
}

// A node pinned to the old derivation has to be retried eventually, or upgrading
// it would never take effect and the migration would never finish.
func TestAPinnedLegacyPeerIsRetriedLater(t *testing.T) {
	now := time.Now()
	p := NewPreference()
	p.now = func() time.Time { return now }
	p.Succeeded("node-1", Legacy256)

	now = now.Add(reprobeAfter / 2)
	if got := p.Next("node-1"); got != Legacy256 {
		t.Errorf("Next = %v too early, want sha256 still", got)
	}
	now = now.Add(reprobeAfter)
	if got := p.Next("node-1"); got != SHA512 {
		t.Errorf("Next = %v after the reprobe window, want sha512", got)
	}
}

// A peer already on the new derivation must never be dropped back to the old one
// by the passage of time.
func TestAPinnedNewPeerIsNeverDowngraded(t *testing.T) {
	now := time.Now()
	p := NewPreference()
	p.now = func() time.Time { return now }
	p.Succeeded("node-1", SHA512)
	now = now.Add(100 * reprobeAfter)
	if got := p.Next("node-1"); got != SHA512 {
		t.Errorf("Next = %v, want sha512 kept", got)
	}
}

func TestPeersAreIndependent(t *testing.T) {
	p := NewPreference()
	p.Succeeded("old", Legacy256)
	p.Succeeded("new", SHA512)
	if p.Next("old") != Legacy256 || p.Next("new") != SHA512 {
		t.Error("one peer's derivation leaked into another's")
	}
	if _, ok := p.Known("never-seen"); ok {
		t.Error("an unseen peer reported as known")
	}
}

// Several connections to one relay discover independently and finish out of
// order. A failure arriving after another connection has already proved the
// derivation must not undo it, or the two flip on every attempt and the peer
// looks unreachable rather than merely old.
func TestAProvenPeerSurvivesALateFailure(t *testing.T) {
	p := NewPreference()
	p.Succeeded("relay", Legacy256)
	p.Failed("relay", Legacy256)

	if got := p.Next("relay"); got != Legacy256 {
		t.Errorf("Next = %v after a late failure, want the proven sha256", got)
	}
	if v, ok := p.Known("relay"); !ok || v != Legacy256 {
		t.Errorf("Known = %v/%v, want the proven sha256 to stand", v, ok)
	}
}

// An unproven guess must still flip, or a peer whose first derivation is wrong
// is never reached at all.
func TestAnUnprovenGuessStillFlips(t *testing.T) {
	p := NewPreference()
	p.Failed("relay", SHA512)
	if got := p.Next("relay"); got != Legacy256 {
		t.Errorf("Next = %v, want the other derivation to be tried", got)
	}
}
