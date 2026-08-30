// Package tokenaead is a gRPC transport that encrypts the connection with
// AES-256-GCM using a key derived from a shared bearer token, instead of X.509
// certificates. Because both peers derive the same keys from the token there is
// NO handshake on the wire - the first bytes are already the (encrypted) HTTP/2
// preface - so it sidesteps the TLS cert-chain that overflows a k8s pod's reduced
// MTU. Possession of the token is both authentication and the encryption key: a
// wrong token cannot decrypt the stream, so the connection simply fails.
//
// The token keys two independent directions (client->server, server->client),
// each an AES-256-GCM AEAD with its own monotonically increasing record counter
// as the nonce, so nonces never repeat under a key. There is no forward secrecy
// (a leaked token decrypts captured traffic); acceptable for a management channel
// already gated by that same token.
package tokenaead

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash"
	"io"
	"net"
	"sync"

	"google.golang.org/grpc/credentials"
)

const (
	keyLen        = 32        // AES-256
	nonceLen      = 12        // GCM standard nonce
	maxPlainChunk = 16 * 1024 // per-record plaintext cap
	protocolName  = "wingsv-token-aes256gcm"
	hkdfSalt      = "wingsv-mgmt-grpc-v1"
)

// Variant selects the hash the key derivation runs on.
//
// The panel's existing links - 3x-ui nodes and vk-turn-proxy relays - derive with
// SHA-256, because a deployed node never stores a raw token, only its SHA-256
// digest, and that digest is the only value the two sides provably share.
// Changing those is a fleet-wide flag day. Links where both ends are new, which
// today means the federation head, use SHA-512 per the project rule.
type Variant int

const (
	// Legacy256 is the zero value because it is what every existing link is
	Legacy256 Variant = 0
	// SHA512 is for links this project mints both ends of
	SHA512 Variant = 1
)

func (v Variant) hash() func() hash.Hash {
	if v == SHA512 {
		return sha512.New
	}
	return sha256.New
}

func deriveKey(v Variant, token []byte, label string) []byte {
	// A non-empty token is required upstream; hkdf.Key never fails for these
	// fixed lengths, so a derivation error is treated as a programming bug.
	key, err := hkdf.Key(v.hash(), token, []byte(hkdfSalt), label, keyLen)
	if err != nil {
		panic("tokenaead: hkdf: " + err.Error())
	}
	return key
}

func newGCM(key []byte) cipher.AEAD {
	block, err := aes.NewCipher(key)
	if err != nil {
		panic("tokenaead: aes: " + err.Error())
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		panic("tokenaead: gcm: " + err.Error())
	}
	return aead
}

// aeadConn wraps a net.Conn, encrypting each write into a length-prefixed
// AES-256-GCM record and decrypting on read. Reads and writes use independent
// AEADs and counters so the two directions never share a nonce space.
type aeadConn struct {
	net.Conn
	writeAEAD, readAEAD cipher.AEAD
	writeCtr, readCtr   uint64
	writeMu, readMu     sync.Mutex
	readBuf             []byte

	// onFirstRead fires once, with whether the first record decrypted. It is the
	// dialing side's only evidence about which derivation the peer speaks
	onFirstRead func(ok bool)
	reported    bool
}

func nonce(counter uint64) []byte {
	n := make([]byte, nonceLen)
	binary.BigEndian.PutUint64(n[nonceLen-8:], counter)
	return n
}

func (a *aeadConn) Write(p []byte) (int, error) {
	a.writeMu.Lock()
	defer a.writeMu.Unlock()
	written := 0
	for len(p) > 0 {
		chunk := p
		if len(chunk) > maxPlainChunk {
			chunk = chunk[:maxPlainChunk]
		}
		ct := a.writeAEAD.Seal(nil, nonce(a.writeCtr), chunk, nil)
		a.writeCtr++
		var hdr [4]byte
		binary.BigEndian.PutUint32(hdr[:], uint32(len(ct)))
		if _, err := a.Conn.Write(hdr[:]); err != nil {
			return written, err
		}
		if _, err := a.Conn.Write(ct); err != nil {
			return written, err
		}
		written += len(chunk)
		p = p[len(chunk):]
	}
	return written, nil
}

// report fires onFirstRead exactly once. Callers hold readMu.
func (a *aeadConn) report(ok bool) {
	if a.reported || a.onFirstRead == nil {
		return
	}
	a.reported = true
	a.onFirstRead(ok)
}

func (a *aeadConn) Read(p []byte) (int, error) {
	a.readMu.Lock()
	defer a.readMu.Unlock()
	if len(a.readBuf) == 0 {
		ct, err := readRecord(a.Conn)
		if err != nil {
			a.report(false)
			return 0, err
		}
		pt, err := a.readAEAD.Open(nil, nonce(a.readCtr), ct, nil)
		a.report(err == nil)
		if err != nil {
			return 0, err
		}
		a.readCtr++
		a.readBuf = pt
	}
	n := copy(p, a.readBuf)
	a.readBuf = a.readBuf[n:]
	return n, nil
}

type authInfo struct {
	credentials.CommonAuthInfo
	variant Variant
}

func (authInfo) AuthType() string { return protocolName }

// String names a derivation for logs.
func (v Variant) String() string {
	if v == SHA512 {
		return "sha512"
	}
	return "sha256"
}

// NegotiatedVariant reports which derivation a connection resolved to.
func NegotiatedVariant(info credentials.AuthInfo) (Variant, bool) {
	ai, ok := info.(authInfo)
	if !ok {
		return Legacy256, false
	}
	return ai.variant, true
}

// Creds is a gRPC TransportCredentials that secures the connection with the
// token-derived AES-256-GCM stream. Use Client on a dialer and Server on a
// listener; both must be built from the same token.
type Creds struct {
	c2s, s2c []byte
	server   bool
	// accept holds every derivation a listener will try. One entry is the fast
	// path; more than one is a fleet mid-migration
	accept []keypair

	// variant, pref and prefKey are the dialing side's half of the migration: the
	// connection reports back whether the peer accepted this derivation
	variant Variant
	pref    *Preference
	prefKey string
}

// ClientFor builds dialer credentials for one peer, choosing the derivation from
// what that peer last answered on and reporting the outcome back.
//
// This is what makes the migration need no configuration anywhere: an upgraded
// node starts being dialled with SHA-512 on its own, and one that has not been
// upgraded costs a single failed attempt before it is remembered.
func ClientFor(token, peerKey string, pref *Preference) *Creds {
	if pref == nil {
		pref = Peers
	}
	v := pref.Next(peerKey)
	c := ClientVariant(token, v)
	c.variant, c.pref, c.prefKey = v, pref, peerKey
	return c
}

// HashSecret maps a raw bearer token onto the shared secret both peers key the
// transport with: its lowercase hex SHA-256 digest.
//
// It exists because 3x-ui never stores the raw token - `x-ui grpc-connect` keeps
// only crypto.HashTokenSHA256(token) - so the digest is the strongest value the
// two sides provably share. The digest must therefore be treated as key material
// wherever it is stored, not as a one-way check value.
func HashSecret(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// Client builds dialer credentials keyed by token.
func Client(token string) *Creds { return ClientVariant(token, Legacy256) }

// ClientVariant builds dialer credentials with an explicit derivation variant.
func ClientVariant(token string, v Variant) *Creds {
	return &Creds{
		c2s: deriveKey(v, []byte(token), "c2s"),
		s2c: deriveKey(v, []byte(token), "s2c"),
	}
}

// Server builds listener credentials keyed by token.
func Server(token string) *Creds { return ServerVariant(token, Legacy256) }

// ServerVariant builds listener credentials with an explicit derivation variant.
func ServerVariant(token string, v Variant) *Creds {
	return &Creds{
		c2s:    deriveKey(v, []byte(token), "c2s"),
		s2c:    deriveKey(v, []byte(token), "s2c"),
		server: true,
	}
}

// ServerAny builds listener credentials that accept any of the given
// derivations, in the order listed. This is what lets a fleet move to SHA-512 a
// node at a time instead of all at once: an updated node keeps serving panels
// that have not moved yet
func ServerAny(token string, variants ...Variant) *Creds {
	if len(variants) == 0 {
		variants = []Variant{Legacy256}
	}
	c := &Creds{server: true}
	for _, v := range variants {
		c.accept = append(c.accept, keypair{
			variant: v,
			c2s:     deriveKey(v, []byte(token), "c2s"),
			s2c:     deriveKey(v, []byte(token), "s2c"),
		})
	}
	c.c2s, c.s2c = c.accept[0].c2s, c.accept[0].s2c
	return c
}

func wrap(raw net.Conn, writeKey, readKey []byte) (net.Conn, credentials.AuthInfo, error) {
	return &aeadConn{Conn: raw, writeAEAD: newGCM(writeKey), readAEAD: newGCM(readKey)},
		authInfo{CommonAuthInfo: credentials.CommonAuthInfo{SecurityLevel: credentials.PrivacyAndIntegrity}}, nil
}

// ClientHandshake wraps raw for the dialing side: it writes c2s, reads s2c.
func (c *Creds) ClientHandshake(_ context.Context, _ string, raw net.Conn) (net.Conn, credentials.AuthInfo, error) {
	conn, info, err := wrap(raw, c.c2s, c.s2c)
	if err != nil || c.pref == nil {
		return conn, info, err
	}
	// A server that cannot open our records just closes the connection, so the
	// first read is the only signal available that the derivation was right.
	ac := conn.(*aeadConn)
	ac.onFirstRead = func(ok bool) {
		if ok {
			c.pref.Succeeded(c.prefKey, c.variant)
			return
		}
		c.pref.Failed(c.prefKey, c.variant)
	}
	return ac, info, err
}

// ServerHandshake wraps raw for the listening side: it writes s2c, reads c2s.
func (c *Creds) ServerHandshake(raw net.Conn) (net.Conn, credentials.AuthInfo, error) {
	if len(c.accept) > 1 {
		return serverHandshakeAny(raw, c.accept)
	}
	return wrap(raw, c.s2c, c.c2s)
}

func (c *Creds) Info() credentials.ProtocolInfo {
	return credentials.ProtocolInfo{SecurityProtocol: protocolName}
}

func (c *Creds) Clone() credentials.TransportCredentials { cc := *c; return &cc }

func (c *Creds) OverrideServerName(string) error { return nil }

// keypair is one derivation's two directions, kept so a server can try several
type keypair struct {
	variant  Variant
	c2s, s2c []byte
}

// ErrNoVariant means the first record opened under none of the accepted
// derivations, which is indistinguishable from a wrong token - and deliberately
// so: the peer learns nothing about which of the two it got wrong
var ErrNoVariant = errors.New("tokenaead: connection did not open under any accepted derivation")

// maxRecordCipher bounds a record's ciphertext. Without it the length prefix,
// which arrives before anything is authenticated, lets an unauthenticated peer
// make the server allocate up to 4 GiB. Both ends must agree on maxPlainChunk;
// the extra 16 bytes are the GCM tag
const maxRecordCipher = maxPlainChunk + 16

// readRecord pulls one length-prefixed record off the wire
func readRecord(conn net.Conn) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(conn, hdr[:]); err != nil {
		return nil, err
	}
	size := binary.BigEndian.Uint32(hdr[:])
	if size == 0 || size > maxRecordCipher {
		return nil, ErrNoVariant
	}
	ct := make([]byte, size)
	if _, err := io.ReadFull(conn, ct); err != nil {
		return nil, err
	}
	return ct, nil
}

// serverHandshakeAny resolves which derivation the peer used.
//
// This transport has no handshake - the first bytes on the wire are already the
// encrypted HTTP/2 preface - so the peer cannot be asked. The server instead
// tries the first record against each accepted derivation; GCM's tag means at
// most one can open, so the answer is unambiguous and costs one AES setup.
//
// It has to happen here rather than lazily on the first Read, because gRPC
// writes its SETTINGS frame before it reads the client preface: by the time a
// read happens the server has already had to encrypt something
func serverHandshakeAny(raw net.Conn, accept []keypair) (net.Conn, credentials.AuthInfo, error) {
	record, err := readRecord(raw)
	if err != nil {
		return nil, nil, err
	}
	for _, kp := range accept {
		readAEAD := newGCM(kp.c2s)
		plain, openErr := readAEAD.Open(nil, nonce(0), record, nil)
		if openErr != nil {
			continue
		}
		conn := &aeadConn{
			Conn:      raw,
			writeAEAD: newGCM(kp.s2c),
			readAEAD:  readAEAD,
			readCtr:   1,
			readBuf:   plain,
		}
		return conn, authInfo{
			CommonAuthInfo: credentials.CommonAuthInfo{SecurityLevel: credentials.PrivacyAndIntegrity},
			variant:        kp.variant,
		}, nil
	}
	return nil, nil, ErrNoVariant
}
