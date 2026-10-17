package app

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
)

//go:embed all:web
var webFS embed.FS

type App struct {
	db      *sql.DB
	mail    Sender
	secret  []byte
	baseURL string
	loc     *time.Location
	now     func() time.Time
	admins  map[string]bool
}

func New(db *sql.DB, mail Sender, secret, baseURL string) *App {
	loc, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		loc = time.FixedZone("BRT", -3*3600)
	}
	return &App{db: db, mail: mail, secret: []byte(secret), baseURL: strings.TrimRight(baseURL, "/"), loc: loc, now: time.Now}
}

func (a *App) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })

	mux.HandleFunc("POST /api/auth/request", a.handleRequestCode)
	mux.HandleFunc("POST /api/auth/verify", a.handleVerify)
	mux.HandleFunc("POST /api/auth/logout", a.auth(a.handleLogout))
	mux.HandleFunc("GET /api/me", a.auth(a.handleMe))
	mux.HandleFunc("GET /api/me/export", a.auth(a.handleExportAccount))
	mux.HandleFunc("DELETE /api/me", a.auth(a.handleDeleteAccount))
	mux.HandleFunc("GET /api/me/key", a.auth(a.handleKeyInfo))
	mux.HandleFunc("POST /api/me/key", a.auth(a.handleKeyCreate))
	mux.HandleFunc("DELETE /api/me/key", a.auth(a.handleKeyDelete))
	mux.HandleFunc("GET /api/admin/stats", a.auth(a.handleAdminStats))

	mux.HandleFunc("GET /api/decks", a.authKey(a.handleListDecks))
	mux.HandleFunc("POST /api/decks", a.authKey(a.handleAddDeck))
	mux.HandleFunc("DELETE /api/decks/{id}", a.auth(a.handleRemoveDeck))
	mux.HandleFunc("GET /api/decks/{id}/status", a.authKey(a.handleDeckStatus))
	mux.HandleFunc("POST /api/decks/{id}/snapshot", a.authKey(a.handleSnapshot))
	mux.HandleFunc("GET /api/decks/{id}/history", a.auth(a.tracked(a.handleDeckHistory)))
	mux.HandleFunc("GET /api/decks/{id}/cards", a.auth(a.tracked(a.handleDeckCards)))
	mux.HandleFunc("GET /api/decks/{id}/card-history", a.auth(a.tracked(a.handleCardHistory)))

	mux.HandleFunc("PUT /api/decks/{id}/alert", a.auth(a.tracked(a.handleSetAlert)))
	mux.HandleFunc("POST /api/decks/{id}/share", a.auth(a.tracked(a.handleShareCreate)))
	mux.HandleFunc("DELETE /api/decks/{id}/share", a.auth(a.tracked(a.handleShareDelete)))
	mux.HandleFunc("GET /api/decks/{id}/export.csv", a.auth(a.tracked(a.handleExportCSV)))

	mux.HandleFunc("GET /api/public/{token}/history", a.public(a.handleDeckHistory))
	mux.HandleFunc("GET /api/public/{token}/cards", a.public(a.handleDeckCards))
	mux.HandleFunc("GET /api/public/{token}/card-history", a.public(a.handleCardHistory))
	mux.HandleFunc("GET /api/public/{token}/export.csv", a.public(a.handleExportCSV))

	mux.HandleFunc("GET /api/compare", a.auth(a.handleCompare))
	mux.HandleFunc("POST /api/transfer", a.auth(a.handleTransfer))
	mux.HandleFunc("GET /api/extension", a.handleExtensionInfo)
	mux.HandleFunc("GET /extensao.zip", a.handleExtensionZip)
	mux.HandleFunc("GET /parse.js", a.handleParseJS)

	sub, _ := fs.Sub(webFS, "web")
	mux.Handle("GET /", http.FileServerFS(sub))
	return headers(cors(mux))
}

// Cleanup apaga códigos e sessões vencidos. Roda na inicialização e a cada 6h.
func (a *App) Cleanup(ctx context.Context) {
	run := func() {
		now := a.now().Unix()
		if _, err := a.db.ExecContext(ctx, `DELETE FROM login_codes WHERE expires_at < ?`, now-86400); err != nil {
			log.Printf("cleanup codes: %v", err)
		}
		if _, err := a.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at < ?`, now); err != nil {
			log.Printf("cleanup sessions: %v", err)
		}
	}
	run()
	t := time.NewTicker(6 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			run()
		}
	}
}

func headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("X-Frame-Options", "DENY")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

// ---- helpers ----

type user struct {
	ID    int64
	Email string
}

type authedHandler func(http.ResponseWriter, *http.Request, user)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func serverErr(w http.ResponseWriter, err error) {
	log.Printf("erro: %v", err)
	writeErr(w, http.StatusInternalServerError, "Erro interno. Tente de novo.")
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		writeErr(w, http.StatusUnsupportedMediaType, "Envie JSON.")
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "JSON inválido.")
		return false
	}
	return true
}

func pathDeckID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

var deckIDRe = regexp.MustCompile(`[?&]id=(\d+)`)

// parseDeckRef aceita "10256897" ou uma URL da LigaMagic com ?id=.
func parseDeckRef(s string) (int64, bool) {
	s = strings.TrimSpace(s)
	if m := deckIDRe.FindStringSubmatch(s); m != nil {
		s = m[1]
	}
	id, err := strconv.ParseInt(s, 10, 64)
	return id, err == nil && id > 0
}

func deckURL(id int64) string {
	return "https://www.ligamagic.com.br/?view=dks/deck&id=" + strconv.FormatInt(id, 10)
}

func (a *App) today() string {
	return a.now().In(a.loc).Format("2006-01-02")
}
