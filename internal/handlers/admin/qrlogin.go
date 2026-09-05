package admin

// Вход по QR: пароль не набирают вовсе. Машина показывает код, а личность
// подтверждает телефон, где человек уже вошёл. Подсмотреть через плечо или
// снять кейлоггером тут нечего

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"v.wingsnet.org/internal/auth"
	"v.wingsnet.org/internal/storage"
)

const (
	// qrTTL - сколько ждём подтверждения. Две минуты: дольше на экране висит
	// код, которым может воспользоваться кто угодно, кто его сфотографировал
	qrTTL = 2 * time.Minute
	// qrSweepEvery - как часто выкидываем протухшие. Кодов единицы, поэтому
	// чистим лениво, прямо при заведении нового
	qrSweepEvery = time.Minute
)

// qrRequest - одна машина, которая просится внутрь
type qrRequest struct {
	createdAt time.Time
	expiresAt time.Time
	// from - кто просится. Показываем это подтверждающему: QR со стороны это
	// чужой QR, и человек должен видеть, куда впускает
	fromIP    string
	userAgent string
	// approvedBy - кого впустили. Ноль означает, что ещё ждём
	approvedBy int64
	// taken - сессию уже забрали. Второй раз по тому же коду не пустим
	taken bool
}

type qrDesk struct {
	mu      sync.Mutex
	rows    map[string]qrRequest
	sweptAt time.Time
}

func newQRDesk() *qrDesk { return &qrDesk{rows: map[string]qrRequest{}} }

func (d *qrDesk) open(fromIP, userAgent string) (string, time.Time, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", time.Time{}, err
	}
	code := base64.RawURLEncoding.EncodeToString(buf)
	now := time.Now()

	d.mu.Lock()
	defer d.mu.Unlock()
	if now.Sub(d.sweptAt) > qrSweepEvery {
		for key, row := range d.rows {
			if now.After(row.expiresAt) {
				delete(d.rows, key)
			}
		}
		d.sweptAt = now
	}
	expires := now.Add(qrTTL)
	d.rows[code] = qrRequest{
		createdAt: now, expiresAt: expires,
		fromIP: fromIP, userAgent: userAgent,
	}
	return code, expires, nil
}

func (d *qrDesk) get(code string) (qrRequest, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	row, ok := d.rows[code]
	if !ok || time.Now().After(row.expiresAt) {
		return qrRequest{}, false
	}
	return row, true
}

func (d *qrDesk) approve(code string, adminID int64) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	row, ok := d.rows[code]
	if !ok || time.Now().After(row.expiresAt) || row.approvedBy != 0 {
		return false
	}
	row.approvedBy = adminID
	d.rows[code] = row
	return true
}

// take отдаёт одобрение ровно один раз и забывает код
func (d *qrDesk) take(code string) (int64, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	row, ok := d.rows[code]
	if !ok || time.Now().After(row.expiresAt) || row.approvedBy == 0 || row.taken {
		return 0, false
	}
	delete(d.rows, code)
	return row.approvedBy, true
}

// handleQRStart заводит код для машины, которая хочет войти
func (h *Handler) handleQRStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	code, expires, err := h.qr.open(clientIP(r), r.UserAgent())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "не смог завести код")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"code": code,
		// Ссылка, а не голый код: системная камера телефона откроет её сама, а
		// приложение поймает ту же ссылку своим сканером
		"url":        strings.TrimRight(h.cfg.PublicBaseURL, "/") + "/link/" + code,
		"expires_at": expires.UTC().Format(timeRFC3339),
	})
}

// handleQRStatus - машина спрашивает, впустили ли её.
//
// Опрос раз в секунду, без сокета: код живёт две минуты, и городить ради них
// постоянное соединение незачем
func (h *Handler) handleQRStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	code := strings.TrimSpace(r.URL.Query().Get("code"))
	if _, ok := h.qr.get(code); !ok {
		writeJSON(w, http.StatusOK, map[string]any{"state": "expired"})
		return
	}
	adminID, ok := h.qr.take(code)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"state": "pending"})
		return
	}
	admin, sess, err := h.auth.OpenSession(adminID)
	if err != nil {
		var suspended *auth.SuspendedError
		if errors.As(err, &suspended) {
			writeJSON(w, http.StatusOK, map[string]any{"state": "refused", "message": suspended.Reason})
			return
		}
		writeError(w, http.StatusInternalServerError, "не смог открыть сессию")
		return
	}
	h.auth.WriteSessionCookie(w, sess)
	_ = h.store.AppendAudit(storage.AuditEntry{
		ActorAdminID: admin.ID, ActorUsername: admin.Username,
		Action: "admin.qr_login", TargetType: "admin",
	})
	writeJSON(w, http.StatusOK, map[string]any{"state": "approved"})
}

// handleQRPending показывает подтверждающему, кого он собирается впустить
func (h *Handler) handleQRPending(w http.ResponseWriter, r *http.Request, _ storage.Admin) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	row, ok := h.qr.get(strings.TrimSpace(r.URL.Query().Get("code")))
	if !ok {
		writeError(w, http.StatusNotFound, "код просрочен или не существует")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"from_ip":    row.fromIP,
		"user_agent": row.userAgent,
		"asked_at":   row.createdAt.UTC().Format(timeRFC3339),
		"approved":   row.approvedBy != 0,
	})
}

type qrApproveRequest struct {
	Code string `json:"code"`
}

// handleQRApprove впускает машину. Только от вошедшего: подтверждение и есть
// вся защита этого входа
func (h *Handler) handleQRApprove(w http.ResponseWriter, r *http.Request, admin storage.Admin) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	var req qrApproveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "bad request")
		return
	}
	if !h.qr.approve(strings.TrimSpace(req.Code), admin.ID) {
		writeError(w, http.StatusNotFound, "код просрочен или уже использован")
		return
	}
	_ = h.store.AppendAudit(storage.AuditEntry{
		ActorAdminID: admin.ID, ActorUsername: admin.Username,
		Action: "admin.qr_approved", TargetType: "admin",
	})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
