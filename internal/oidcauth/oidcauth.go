// Package matrixauth signs admins in with the deployment's own Matrix account
// service.
//
// One homeserver and no other. That is the whole security property: the invite
// tree only resists sybils because an identity costs something, and accepting
// logins from any homeserver on the internet would let anybody mint as many
// identities as they felt like on a server they run themselves.
package oidcauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Config is what the deployment knows about its own account service.
type Config struct {
	// Issuer is the MAS base URL. Pinned, and pinning it is what restricts logins
	// to our own homeserver: no other issuer can mint a token this client accepts.
	Issuer string
	// Homeserver is the domain in an MXID, checked again after the exchange in
	// case MAS is ever pointed at more than one.
	Homeserver   string
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

// Enabled reports whether the deployment configured Matrix login at all.
func (c Config) Enabled() bool {
	return strings.TrimSpace(c.Issuer) != "" &&
		strings.TrimSpace(c.ClientID) != "" &&
		strings.TrimSpace(c.Homeserver) != ""
}

// Identity is who signed in.
type Identity struct {
	// MatrixID is the full @localpart:homeserver form. It is the account's
	// identity here; the OIDC subject is kept only to notice a rename.
	MatrixID    string
	Localpart   string
	Subject     string
	DisplayName string
	AvatarURL   string
}

// pendingTTL bounds how long a started login stays valid. Short: it covers one
// redirect to a service the user is already signed in to.
const pendingTTL = 10 * time.Minute

// pending is one in-flight login.
type pending struct {
	verifier string
	returnTo string
	// linkAdminID is set when an existing admin is attaching a Matrix account
	// rather than signing in with one.
	linkAdminID int64
	// invite is carried through the round trip because the provider gives back
	// only state and code. Without it a first-time visitor arrives at the
	// callback with no code in hand and cannot be registered at all.
	invite string
	at     time.Time
}

// Client drives the login flow.
type Client struct {
	cfg Config

	mu       sync.Mutex
	provider *oidc.Provider
	oauth    *oauth2.Config
	verifier *oidc.IDTokenVerifier
	states   map[string]pending
}

// New builds a client. Discovery is deferred: a panel must start even when the
// account service is down, or one restart of MAS takes the panel with it.
func New(cfg Config) *Client {
	return &Client{cfg: cfg, states: map[string]pending{}}
}

// Enabled reports whether Matrix login is configured.
func (c *Client) Enabled() bool { return c.cfg.Enabled() }

// ErrDisabled means no account service is configured.
var ErrDisabled = errors.New("oidcauth: matrix login is not configured")

// ErrForeignHomeserver means the identity is not ours. Refusing it is the point:
// an identity from somewhere else costs nothing to mint.
var ErrForeignHomeserver = errors.New("oidcauth: identity is not from this homeserver")

// ErrUnknownState means the callback does not match a login this panel started.
var ErrUnknownState = errors.New("oidcauth: unknown or expired login")

func (c *Client) discover(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.provider != nil {
		return nil
	}
	// Issuer передаётся как есть: библиотека сверяет его с тем, что вернул
	// провайдер, посимвольно, и лишний или срезанный слеш ломает вход
	provider, err := oidc.NewProvider(ctx, c.cfg.Issuer)
	if err != nil {
		return err
	}
	c.provider = provider
	c.verifier = provider.Verifier(&oidc.Config{ClientID: c.cfg.ClientID})
	c.oauth = &oauth2.Config{
		ClientID:     c.cfg.ClientID,
		ClientSecret: c.cfg.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  c.cfg.RedirectURL,
		Scopes:       []string{oidc.ScopeOpenID, "profile"},
	}
	return nil
}

// Start returns the URL to send the browser to.
//
// PKCE even though this is a confidential client: the code travels through a
// browser redirect either way, and the extra binding costs nothing.
func (c *Client) Start(ctx context.Context, returnTo string, linkAdminID int64, invite string) (string, error) {
	if !c.cfg.Enabled() {
		return "", ErrDisabled
	}
	if err := c.discover(ctx); err != nil {
		return "", err
	}
	state, err := randomString()
	if err != nil {
		return "", err
	}
	verifier, err := randomString()
	if err != nil {
		return "", err
	}
	challenge := sha256.Sum256([]byte(verifier))

	c.mu.Lock()
	c.sweepLocked()
	c.states[state] = pending{
		verifier: verifier, returnTo: returnTo, linkAdminID: linkAdminID,
		invite: invite, at: time.Now(),
	}
	oauthCfg := c.oauth
	c.mu.Unlock()

	return oauthCfg.AuthCodeURL(state,
		oauth2.SetAuthURLParam("code_challenge", base64.RawURLEncoding.EncodeToString(challenge[:])),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	), nil
}

// Complete exchanges the code and returns who signed in.
func (c *Client) Complete(ctx context.Context, state, code string) (Identity, string, int64, string, error) {
	if !c.cfg.Enabled() {
		return Identity{}, "", 0, "", ErrDisabled
	}
	if err := c.discover(ctx); err != nil {
		return Identity{}, "", 0, "", err
	}

	c.mu.Lock()
	c.sweepLocked()
	got, ok := c.states[state]
	delete(c.states, state)
	oauthCfg, verifier := c.oauth, c.verifier
	c.mu.Unlock()
	if !ok {
		return Identity{}, "", 0, "", ErrUnknownState
	}

	token, err := oauthCfg.Exchange(ctx, code,
		oauth2.SetAuthURLParam("code_verifier", got.verifier))
	if err != nil {
		return Identity{}, "", 0, "", err
	}
	rawID, ok := token.Extra("id_token").(string)
	if !ok {
		return Identity{}, "", 0, "", errors.New("oidcauth: no id token in the response")
	}
	idToken, err := verifier.Verify(ctx, rawID)
	if err != nil {
		return Identity{}, "", 0, "", err
	}

	var claims struct {
		Subject           string `json:"sub"`
		PreferredUsername string `json:"preferred_username"`
		Name              string `json:"name"`
		Picture           string `json:"picture"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return Identity{}, "", 0, "", err
	}
	identity, err := c.identityFrom(claims.Subject, claims.PreferredUsername, claims.Name, claims.Picture)
	if err != nil {
		return Identity{}, "", 0, "", err
	}
	return identity, got.returnTo, got.linkAdminID, got.invite, nil
}

// identityFrom builds the MXID and refuses anything that is not ours.
func (c *Client) identityFrom(subject, localpart, name, picture string) (Identity, error) {
	localpart = strings.TrimSpace(localpart)
	if localpart == "" {
		return Identity{}, errors.New("oidcauth: the account service returned no username")
	}
	// MAS can hand back either a bare localpart or a full MXID depending on how
	// it is configured, so normalise before checking the domain
	mxid := localpart
	if !strings.HasPrefix(mxid, "@") {
		mxid = "@" + localpart + ":" + c.cfg.Homeserver
	}
	at, domain, found := strings.Cut(strings.TrimPrefix(mxid, "@"), ":")
	if !found || at == "" {
		return Identity{}, fmt.Errorf("oidcauth: %q is not a matrix id", mxid)
	}
	if !strings.EqualFold(domain, c.cfg.Homeserver) {
		return Identity{}, ErrForeignHomeserver
	}
	return Identity{
		MatrixID:    "@" + at + ":" + strings.ToLower(domain),
		Localpart:   at,
		Subject:     subject,
		DisplayName: strings.TrimSpace(name),
		AvatarURL:   strings.TrimSpace(picture),
	}, nil
}

// sweepLocked drops logins nobody came back from. Callers hold the lock.
func (c *Client) sweepLocked() {
	cutoff := time.Now().Add(-pendingTTL)
	for state, p := range c.states {
		if p.at.Before(cutoff) {
			delete(c.states, state)
		}
	}
}

func randomString() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
