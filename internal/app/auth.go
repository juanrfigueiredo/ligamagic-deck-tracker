package app

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/mail"
	"strings"
	"time"
)

const (
	codeTTL         = 10 * time.Minute
	sessionTTL      = 30 * 24 * time.Hour
	maxAttempts     = 5
	resendCooldown  = 60 * time.Second
	maxCodesPerHour = 5
	cookieName      = "mtg_session"
)

func normEmail(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if len(s) > 254 {
		return "", false
	}
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Address != s || !strings.Contains(s[strings.LastIndex(s, "@"):], ".") {
		return "", false
	}
	return s, true
}

func (a *App) hashCode(email, code string) string {
	m := hmac.New(sha256.New, a.secret)
	m.Write([]byte(email + ":" + code))
	return hex.EncodeToString(m.Sum(nil))
}

func hashToken(raw string) string {
	s := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(s[:])
}

func newCode() string {
	n, _ := rand.Int(rand.Reader, big.NewInt(1_000_000))
	return fmt.Sprintf("%06d", n.Int64())
}

func newToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// POST /api/auth/request {email}
func (a *App) handleRequestCode(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	email, ok := normEmail(in.Email)
	if !ok {
		writeErr(w, http.StatusBadRequest, "E-mail inválido.")
		return
	}
	ctx := r.Context()
	now := a.now()

	var last sql.NullInt64
	var count int
	err := a.db.QueryRowContext(ctx,
		`SELECT MAX(created_at), COUNT(*) FROM login_codes WHERE email = ? AND created_at > ?`,
		email, now.Add(-time.Hour).Unix()).Scan(&last, &count)
	if err != nil {
		serverErr(w, err)
		return
	}
	if last.Valid && now.Unix()-last.Int64 < int64(resendCooldown.Seconds()) {
		writeErr(w, http.StatusTooManyRequests, "Aguarde um minuto para pedir outro código.")
		return
	}
	if count >= maxCodesPerHour {
		writeErr(w, http.StatusTooManyRequests, "Muitos pedidos. Tente de novo em uma hora.")
		return
	}

	code := newCode()
	// Só o código mais recente vale.
	if _, err := a.db.ExecContext(ctx, `UPDATE login_codes SET used_at = ? WHERE email = ? AND used_at IS NULL`, now.Unix(), email); err != nil {
		serverErr(w, err)
		return
	}
	if _, err := a.db.ExecContext(ctx,
		`INSERT INTO login_codes (email, code_hash, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		email, a.hashCode(email, code), now.Unix(), now.Add(codeTTL).Unix()); err != nil {
		serverErr(w, err)
		return
	}

	text := fmt.Sprintf("Seu código de acesso ao MTG Tracker é %s.\n\nEle vale por 10 minutos e só pode ser usado uma vez. Se não foi você que pediu, ignore este e-mail.", code)
	html := fmt.Sprintf(`<p>Seu código de acesso ao <b>MTG Tracker</b>:</p><p style="font-size:28px;font-weight:bold;letter-spacing:4px">%s</p><p>Vale por 10 minutos e só pode ser usado uma vez. Se não foi você que pediu, ignore este e-mail.</p>`, code)
	if err := a.mail.Send(ctx, email, "Seu código: "+code, text, html); err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// POST /api/auth/verify {email, code} -> {token, email, expires_at}
func (a *App) handleVerify(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	email, ok := normEmail(in.Email)
	code := strings.TrimSpace(in.Code)
	if !ok || len(code) != 6 {
		writeErr(w, http.StatusBadRequest, "Código inválido ou expirado.")
		return
	}
	ctx := r.Context()
	now := a.now()

	var id, expires int64
	var attempts int
	var hash string
	err := a.db.QueryRowContext(ctx,
		`SELECT id, code_hash, expires_at, attempts FROM login_codes
		 WHERE email = ? AND used_at IS NULL ORDER BY id DESC LIMIT 1`, email).
		Scan(&id, &hash, &expires, &attempts)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (expires < now.Unix() || attempts >= maxAttempts)) {
		writeErr(w, http.StatusBadRequest, "Código inválido ou expirado. Peça um novo.")
		return
	}
	if err != nil {
		serverErr(w, err)
		return
	}
	if !hmac.Equal([]byte(hash), []byte(a.hashCode(email, code))) {
		a.db.ExecContext(ctx, `UPDATE login_codes SET attempts = attempts + 1 WHERE id = ?`, id)
		left := maxAttempts - attempts - 1
		if left <= 0 {
			writeErr(w, http.StatusBadRequest, "Código incorreto. Peça um novo.")
		} else {
			writeErr(w, http.StatusBadRequest, fmt.Sprintf("Código incorreto. Restam %d tentativas.", left))
		}
		return
	}
	// Uso único: só um pedido consegue marcar como usado.
	res, err := a.db.ExecContext(ctx, `UPDATE login_codes SET used_at = ? WHERE id = ? AND used_at IS NULL`, now.Unix(), id)
	if err != nil {
		serverErr(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n != 1 {
		writeErr(w, http.StatusBadRequest, "Código já usado. Peça um novo.")
		return
	}

	uid, err := a.upsertUser(r, email)
	if err != nil {
		serverErr(w, err)
		return
	}
	token := newToken()
	exp := now.Add(sessionTTL)
	if _, err := a.db.ExecContext(ctx,
		`INSERT INTO sessions (token_hash, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		hashToken(token), uid, now.Unix(), exp.Unix()); err != nil {
		serverErr(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    token,
		Path:     "/",
		Expires:  exp,
		HttpOnly: true,
		Secure:   r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https",
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "email": email, "expires_at": exp.Unix()})
}

func (a *App) upsertUser(r *http.Request, email string) (int64, error) {
	ctx := r.Context()
	if _, err := a.db.ExecContext(ctx,
		`INSERT INTO users (email, created_at) VALUES (?, ?) ON CONFLICT (email) DO NOTHING`,
		email, a.now().Unix()); err != nil {
		return 0, err
	}
	var id int64
	err := a.db.QueryRowContext(ctx, `SELECT id FROM users WHERE email = ?`, email).Scan(&id)
	return id, err
}

func tokenFrom(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(h[7:])
	}
	if c, err := r.Cookie(cookieName); err == nil {
		return c.Value
	}
	return ""
}

func (a *App) auth(next authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tok := tokenFrom(r)
		if tok == "" {
			writeErr(w, http.StatusUnauthorized, "Faça login.")
			return
		}
		var u user
		err := a.db.QueryRowContext(r.Context(),
			`SELECT u.id, u.email FROM sessions s JOIN users u ON u.id = s.user_id
			 WHERE s.token_hash = ? AND s.expires_at > ?`, hashToken(tok), a.now().Unix()).
			Scan(&u.ID, &u.Email)
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, http.StatusUnauthorized, "Sessão expirada. Faça login de novo.")
			return
		}
		if err != nil {
			serverErr(w, err)
			return
		}
		next(w, r, u)
	}
}

func (a *App) handleLogout(w http.ResponseWriter, r *http.Request, u user) {
	a.db.ExecContext(r.Context(), `DELETE FROM sessions WHERE token_hash = ?`, hashToken(tokenFrom(r)))
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) handleMe(w http.ResponseWriter, r *http.Request, u user) {
	writeJSON(w, http.StatusOK, map[string]any{"email": u.Email, "admin": a.isAdmin(u)})
}
