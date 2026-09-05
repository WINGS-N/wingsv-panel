// Package oidcauth signs admins in with the deployment's own account service.
//
// One issuer and no other. That is the whole security property: the invite tree
// only resists sybils because an identity costs something, and accepting logins
// from any provider on the internet would let anybody mint as many identities as
// they felt like on a server they run themselves.
package oidcauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Config is what the deployment knows about its own account service.
type Config struct {
	// Issuer is the account service base URL. Pinned, and pinning it is what keeps
	// logins to our own provider: no other issuer can mint a token this client
	// accepts.
	Issuer       string
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

// Enabled reports whether the deployment configured account login at all.
//
// Ничего не настроено - панель просто живёт на своём пароле. Админ, поднявший её
// у себя, про OIDC так и не узнает, и это намеренно
func (c Config) Enabled() bool {
	return strings.TrimSpace(c.Issuer) != "" && strings.TrimSpace(c.ClientID) != ""
}

// Identity is who signed in.
type Identity struct {
	// Subject - кто это по версии провайдера. Он и есть личность: имя человек
	// меняет когда захочет, а subject выдаётся один раз
	Subject string
	// Username - предложенное имя. Годится только на завод нового админа, дальше
	// решает Subject
	Username    string
	DisplayName string
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
var ErrDisabled = errors.New("oidcauth: account login is not configured")

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
		// Только openid: у MAS нет скоупа profile, и запрос на него отбивается
		// политикой ещё на согласии - причём страницу отказа он же не может
		// отрисовать, так что наружу это выглядит как 500 без объяснений.
		// Имя, localpart и аватар приезжают claim-ами в id_token и так.
		Scopes: []string{oidc.ScopeOpenID},
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
	}
	if err := idToken.Claims(&claims); err != nil {
		return Identity{}, "", 0, "", err
	}
	// MAS кладёт в id_token только служебные claim-ы: в его discovery
	// claims_supported это iss/sub/aud/iat/exp и хеши, без preferred_username.
	// Имя приходится добирать из userinfo - штатный путь OIDC, который заодно
	// не зависит от того, что конкретный провайдер решил положить в токен.
	if strings.TrimSpace(claims.PreferredUsername) == "" {
		if info, infoErr := c.userInfo(ctx, oauthCfg, token); infoErr == nil {
			if claims.PreferredUsername == "" {
				claims.PreferredUsername = firstNonEmpty(info.PreferredUsername, info.Username)
			}
			if claims.Name == "" {
				claims.Name = info.Name
			}
		} else {
			log.Printf("oidcauth: userinfo unavailable: %v", infoErr)
		}
	}
	identity, err := c.identityFrom(claims.Subject, claims.PreferredUsername, claims.Name)
	if err != nil {
		return Identity{}, "", 0, "", err
	}
	return identity, got.returnTo, got.linkAdminID, got.invite, nil
}

// identityFrom собирает личность из того, что отдал провайдер.
//
// Имя чистим до локальной части: провайдер зовёт человека вида admin@org.домен,
// а у нас имена короткие, и тащить в них домен незачем
func (c *Client) identityFrom(subject, username, name string) (Identity, error) {
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return Identity{}, errors.New("oidcauth: the account service returned no subject")
	}
	username = strings.TrimSpace(username)
	if at := strings.IndexByte(username, '@'); at > 0 {
		username = username[:at]
	}
	username = strings.TrimPrefix(username, "@")
	return Identity{
		Subject:     subject,
		Username:    strings.ToLower(username),
		DisplayName: strings.TrimSpace(name),
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

// userInfoClaims is what the userinfo endpoint may carry. MAS spells the
// username as "username"; the OIDC-standard name is preferred_username, and
// both are read rather than betting on one.
type userInfoClaims struct {
	PreferredUsername string `json:"preferred_username"`
	Username          string `json:"username"`
	Name              string `json:"name"`
}

func (c *Client) userInfo(ctx context.Context, oauthCfg *oauth2.Config, token *oauth2.Token) (userInfoClaims, error) {
	var out userInfoClaims
	c.mu.Lock()
	provider := c.provider
	c.mu.Unlock()
	if provider == nil {
		return out, errors.New("oidcauth: no provider")
	}
	info, err := provider.UserInfo(ctx, oauthCfg.TokenSource(ctx, token))
	if err != nil {
		return out, err
	}
	if err := info.Claims(&out); err != nil {
		return out, err
	}
	return out, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
