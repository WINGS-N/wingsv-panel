// Package accountsession водит человека через вход, не отдавая его чужому экрану.
//
// Провайдер умеет показывать свою страницу логина, но она выглядит как чужая
// админка. Поэтому панель сама играет роль экрана входа: начинает запрос
// авторизации, проверяет пароль и второй фактор через API сессий и сама же его
// завершает. Наружу это остаётся обычным OIDC - меняется только то, кто рисует
// форму
package accountsession

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// requestTimeout - один заход к провайдеру. Человек ждёт форму, а не вечность
const requestTimeout = 10 * time.Second

// ErrDisabled - панель не настроена на сервис учёток
var ErrDisabled = errors.New("accountsession: the account service is not configured")

// ErrBadPassword - логин или пароль не подошли. Одна ошибка на оба случая:
// раздельные ответы говорят чужому, какие имена существуют
var ErrBadPassword = errors.New("accountsession: wrong login or password")

// ErrNeedSecondFactor - пароль принят, но нужен код. Не ошибка, а половина пути
var ErrNeedSecondFactor = errors.New("accountsession: a second factor is required")

// Config - что панель знает о сервисе учёток
type Config struct {
	// Issuer - база провайдера, та же, что у OIDC-клиента
	Issuer string
	// Token - PAT сервисного пользователя. Им панель и создаёт сессии: своего
	// пароля она не видит никогда, проверяет его провайдер
	Token string
}

func (c Config) Enabled() bool {
	return strings.TrimSpace(c.Issuer) != "" && strings.TrimSpace(c.Token) != ""
}

// Client ходит в API сессий
type Client struct {
	cfg  Config
	http *http.Client
}

func New(cfg Config) *Client {
	return &Client{cfg: cfg, http: &http.Client{Timeout: requestTimeout}}
}

func (c *Client) Enabled() bool { return c.cfg.Enabled() }

// Session - начатый вход
type Session struct {
	ID    string
	Token string
}

// AuthRequestID достаёт номер запроса авторизации из адреса, куда провайдер
// отправил браузер.
//
// Панель не идёт по этому адресу: там живёт чужой экран, а форму рисуем мы
func AuthRequestID(target string) (string, error) {
	parsed, err := url.Parse(target)
	if err != nil {
		return "", err
	}
	id := strings.TrimSpace(parsed.Query().Get("authRequest"))
	if id == "" {
		return "", fmt.Errorf("accountsession: %q carries no auth request", target)
	}
	return id, nil
}

// Begin спрашивает провайдера, каким номером он завёл этот вход.
//
// Ходим сами, без браузера: ответ - редирект на экран входа, и нужен из него
// только номер
func (c *Client) Begin(ctx context.Context, authURL string) (string, error) {
	if !c.cfg.Enabled() {
		return "", ErrDisabled
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, authURL, nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{
		Timeout: requestTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	location := resp.Header.Get("Location")
	if location == "" {
		return "", fmt.Errorf("accountsession: the provider answered %d without a redirect", resp.StatusCode)
	}
	return AuthRequestID(location)
}

// Password проверяет логин с паролем и заводит сессию.
//
// Пароль уходит провайдеру и нигде у нас не оседает: панель его не хранит и не
// умеет проверять
func (c *Client) Password(ctx context.Context, loginName, password string) (Session, error) {
	body := map[string]any{
		"checks": map[string]any{
			"user":     map[string]any{"loginName": loginName},
			"password": map[string]any{"password": password},
		},
	}
	var out struct {
		SessionID    string `json:"sessionId"`
		SessionToken string `json:"sessionToken"`
	}
	if err := c.call(ctx, http.MethodPost, "/v2/sessions", body, &out); err != nil {
		return Session{}, err
	}
	return Session{ID: out.SessionID, Token: out.SessionToken}, nil
}

// SecondFactor добавляет к сессии код из аутентификатора
func (c *Client) SecondFactor(ctx context.Context, session Session, code string) (Session, error) {
	body := map[string]any{
		"sessionToken": session.Token,
		"checks":       map[string]any{"totp": map[string]any{"code": code}},
	}
	var out struct {
		SessionToken string `json:"sessionToken"`
	}
	if err := c.call(ctx, http.MethodPatch, "/v2/sessions/"+url.PathEscape(session.ID), body, &out); err != nil {
		return Session{}, err
	}
	return Session{ID: session.ID, Token: out.SessionToken}, nil
}

// Finish отдаёт сессию запросу авторизации и получает адрес, куда вернуть
// браузер. Дальше всё идёт обычным путём OIDC, через наш же колбэк
func (c *Client) Finish(ctx context.Context, authRequestID string, session Session) (string, error) {
	body := map[string]any{
		"session": map[string]any{
			"sessionId":    session.ID,
			"sessionToken": session.Token,
		},
	}
	var out struct {
		CallbackURL string `json:"callbackUrl"`
	}
	path := "/v2/oidc/auth_requests/" + url.PathEscape(authRequestID)
	if err := c.call(ctx, http.MethodPost, path, body, &out); err != nil {
		return "", err
	}
	if out.CallbackURL == "" {
		return "", errors.New("accountsession: the provider returned no callback")
	}
	return out.CallbackURL, nil
}

// call - один запрос к API. Ошибку провайдера разворачиваем в свою: наружу
// должно уходить "логин или пароль не подошли", а не его внутренний код
func (c *Client) call(ctx context.Context, method, path string, body, out any) error {
	if !c.cfg.Enabled() {
		return ErrDisabled
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method,
		strings.TrimRight(c.cfg.Issuer, "/")+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	// Любой успех, а не ровно 200: создание сессии провайдер отдаёт как 201, и
	// сверка с одним кодом ломает вход целиком
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var failure struct {
			Message string `json:"message"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&failure)
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
			return ErrBadPassword
		case http.StatusPreconditionFailed:
			return ErrNeedSecondFactor
		case http.StatusBadRequest:
			// Неверный пароль провайдер считает кривым запросом. Отличаем по
			// тексту: у остальных четырёхсотых причина другая, и валить их в
			// "пароль не подошёл" значит врать человеку
			if strings.Contains(strings.ToLower(failure.Message), "password") ||
				strings.Contains(strings.ToLower(failure.Message), "user") {
				return ErrBadPassword
			}
		}
		return fmt.Errorf("accountsession: the provider answered %d: %s", resp.StatusCode, failure.Message)
	}
	if out == nil {
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
