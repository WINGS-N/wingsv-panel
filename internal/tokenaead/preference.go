package tokenaead

import (
	"sync"
	"time"
)

// reprobeAfter is how long a peer stays pinned to the old derivation before the
// panel tries the new one again. Without it a node that upgrades would keep
// being dialled the old way forever, and the migration would never finish.
const reprobeAfter = time.Hour

// Preference records which derivation each peer answered on.
//
// The transport has no handshake, so a peer cannot be asked - it can only be
// tried. That makes a per-peer memory the whole migration mechanism: dial the
// new derivation, and if the peer turns out not to speak it yet, remember and
// stop paying for the discovery on every call.
type Preference struct {
	mu    sync.Mutex
	state map[string]prefEntry
	now   func() time.Time
}

type prefEntry struct {
	variant Variant
	// at is when this was last decided, which is what bounds how long a peer
	// stays on the old derivation after it has been upgraded.
	at time.Time
	// proven means the peer actually answered on this derivation, as opposed to
	// it being the guess left over from a failure.
	proven bool
}

// NewPreference builds an empty record.
func NewPreference() *Preference {
	return &Preference{state: map[string]prefEntry{}, now: time.Now}
}

// Peers is the process-wide record. It has to outlive any one client: the
// collector builds a fresh relay client on every poll, and a memory that died
// with it would rediscover the same answer every three seconds.
var Peers = NewPreference()

// Next is the derivation to try for a peer. New peers get SHA-512, which is
// where the fleet is going; a peer known to be older gets what it answered on,
// until it is due to be tried again.
func (p *Preference) Next(key string) Variant {
	p.mu.Lock()
	defer p.mu.Unlock()
	entry, ok := p.state[key]
	if !ok {
		return SHA512
	}
	if entry.variant == Legacy256 && p.now().Sub(entry.at) > reprobeAfter {
		return SHA512
	}
	return entry.variant
}

// Succeeded records that a peer accepted this derivation.
func (p *Preference) Succeeded(key string, v Variant) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.state[key] = prefEntry{variant: v, at: p.now(), proven: true}
}

// Failed records that a peer did not accept this derivation, so the next
// attempt uses the other one.
//
// A peer that has already answered on a derivation keeps it. Several
// connections to one relay are open at once - the collector and two stat
// streams - and they discover independently, so a late failure from one of them
// would otherwise undo what another had just proved. The pair would then flip on
// every attempt and the connection would never settle, which looks exactly like
// an unreachable peer. A peer that really has been upgraded is still picked up,
// through the reprobe in Next rather than through a failure here.
func (p *Preference) Failed(key string, v Variant) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if entry, ok := p.state[key]; ok && entry.proven {
		return
	}
	other := Legacy256
	if v == Legacy256 {
		other = SHA512
	}
	p.state[key] = prefEntry{variant: other, at: p.now()}
}

// Known reports what a peer last answered on, for an operator asking how far the
// migration has got.
func (p *Preference) Known(key string) (Variant, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	entry, ok := p.state[key]
	return entry.variant, ok
}
