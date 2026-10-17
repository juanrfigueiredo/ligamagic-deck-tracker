package app

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"
)

// Chave de coleta: o favorito (bookmarklet) e o coletor do computador mandam
// "Authorization: Bearer lmk_..." e só podem usar as rotas de coleta.
const keyPrefix = "lmk_"

// Origens da LigaMagic: o favorito roda lá e chama a API direto (CORS, sem cookie).
var corsOrigins = map[string]bool{
	"https://www.ligamagic.com.br": true,
	"https://ligamagic.com.br":     true,
}

// authKey aceita a sessão normal ou a chave de coleta.
func (a *App) authKey(next authedHandler) http.HandlerFunc {
	session := a.auth(next)
	return func(w http.ResponseWriter, r *http.Request) {
		tok := tokenFrom(r)
		if !strings.HasPrefix(tok, keyPrefix) {
			session(w, r)
			return
		}
		var u user
		err := a.db.QueryRowContext(r.Context(),
			`SELECT u.id, u.email FROM api_keys k JOIN users u ON u.id = k.user_id WHERE k.key_hash = ?`,
			hashToken(tok)).Scan(&u.ID, &u.Email)
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, http.StatusUnauthorized, "Chave de coleta inválida. Gere outra no site, em Ajuda.")
			return
		}
		if err != nil {
			serverErr(w, err)
			return
		}
		a.db.ExecContext(r.Context(), `UPDATE api_keys SET last_used_at = ? WHERE user_id = ?`, a.now().Unix(), u.ID)
		next(w, r, u)
	}
}

// GET /api/me/key -> {exists, created_at, last_used_at}
func (a *App) handleKeyInfo(w http.ResponseWriter, r *http.Request, u user) {
	var created int64
	var used sql.NullInt64
	err := a.db.QueryRowContext(r.Context(), `SELECT created_at, last_used_at FROM api_keys WHERE user_id = ?`, u.ID).Scan(&created, &used)
	if errors.Is(err, sql.ErrNoRows) {
		writeJSON(w, http.StatusOK, map[string]any{"exists": false})
		return
	}
	if err != nil {
		serverErr(w, err)
		return
	}
	out := map[string]any{"exists": true, "created_at": a.fmtTime(created), "last_used_at": nil}
	if used.Valid {
		out["last_used_at"] = a.fmtTime(used.Int64)
	}
	writeJSON(w, http.StatusOK, out)
}

// POST /api/me/key -> {key}. Gera uma chave nova e invalida a anterior. A chave só aparece aqui.
func (a *App) handleKeyCreate(w http.ResponseWriter, r *http.Request, u user) {
	key := keyPrefix + newToken()
	_, err := a.db.ExecContext(r.Context(),
		`INSERT INTO api_keys (key_hash, user_id, created_at) VALUES (?, ?, ?)
		 ON CONFLICT (user_id) DO UPDATE SET key_hash = excluded.key_hash, created_at = excluded.created_at, last_used_at = NULL`,
		hashToken(key), u.ID, a.now().Unix())
	if err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"key": key})
}

// DELETE /api/me/key
func (a *App) handleKeyDelete(w http.ResponseWriter, r *http.Request, u user) {
	if _, err := a.db.ExecContext(r.Context(), `DELETE FROM api_keys WHERE user_id = ?`, u.ID); err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) fmtTime(unix int64) string {
	return time.Unix(unix, 0).In(a.loc).Format("02/01/2006 15:04")
}

// cors libera as rotas de coleta para a LigaMagic (o favorito chama a API de lá).
// Sem credenciais: a autenticação é a chave no header.
func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if corsOrigins[origin] && strings.HasPrefix(r.URL.Path, "/api/decks") {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Add("Vary", "Origin")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			h.Set("Access-Control-Allow-Methods", "GET, POST")
			h.Set("Access-Control-Max-Age", "86400")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
