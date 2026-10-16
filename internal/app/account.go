package app

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// SetAdmins define os e-mails (separados por vírgula) que veem o painel de uso em #/admin.
func (a *App) SetAdmins(list string) {
	a.admins = map[string]bool{}
	for _, e := range strings.Split(list, ",") {
		if e, ok := normEmail(e); ok {
			a.admins[e] = true
		}
	}
}

func (a *App) isAdmin(u user) bool { return a.admins[u.Email] }

// ---- dados da conta (LGPD) ----

// GET /api/me/export: tudo o que está guardado sobre a conta, em JSON.
// O histórico de preços não é pessoal: é do deck e fica para quem mais o acompanha.
func (a *App) handleExportAccount(w http.ResponseWriter, r *http.Request, u user) {
	ctx := r.Context()
	ts := func(sec int64) string { return time.Unix(sec, 0).In(a.loc).Format(time.RFC3339) }

	var created int64
	if err := a.db.QueryRowContext(ctx, `SELECT created_at FROM users WHERE id = ?`, u.ID).Scan(&created); err != nil {
		serverErr(w, err)
		return
	}
	type deckOut struct {
		ID        int64   `json:"id"`
		Name      string  `json:"name"`
		URL       string  `json:"url"`
		Since     string  `json:"tracking_since"`
		AlertPct  *int64  `json:"alert_pct"`
		PublicURL *string `json:"public_url"`
	}
	decks := []deckOut{}
	rows, err := a.db.QueryContext(ctx, `
		SELECT d.id, d.name, ud.created_at, al.pct, sh.token FROM user_decks ud
		JOIN decks d ON d.id = ud.deck_id
		LEFT JOIN alerts al ON al.user_id = ud.user_id AND al.deck_id = ud.deck_id
		LEFT JOIN shares sh ON sh.user_id = ud.user_id AND sh.deck_id = ud.deck_id
		WHERE ud.user_id = ? ORDER BY ud.created_at`, u.ID)
	if err != nil {
		serverErr(w, err)
		return
	}
	for rows.Next() {
		var d deckOut
		var since int64
		var pct sql.NullInt64
		var token sql.NullString
		if err := rows.Scan(&d.ID, &d.Name, &since, &pct, &token); err != nil {
			rows.Close()
			serverErr(w, err)
			return
		}
		d.URL, d.Since, d.AlertPct = deckURL(d.ID), ts(since), nullPtr(pct)
		if token.Valid {
			s := a.shareURL(token.String)
			d.PublicURL = &s
		}
		decks = append(decks, d)
	}
	rows.Close()

	type sessOut struct {
		Created string `json:"created_at"`
		Expires string `json:"expires_at"`
	}
	sessions := []sessOut{}
	rows, err = a.db.QueryContext(ctx, `SELECT created_at, expires_at FROM sessions WHERE user_id = ? ORDER BY created_at`, u.ID)
	if err != nil {
		serverErr(w, err)
		return
	}
	for rows.Next() {
		var c, e int64
		if rows.Scan(&c, &e) == nil {
			sessions = append(sessions, sessOut{ts(c), ts(e)})
		}
	}
	rows.Close()

	w.Header().Set("Content-Disposition", `attachment; filename="mtg-tracker-meus-dados.json"`)
	writeJSON(w, http.StatusOK, map[string]any{
		"exported_at": ts(a.now().Unix()),
		"email":       u.Email,
		"created_at":  ts(created),
		"decks":       decks,
		"sessions":    sessions,
		"note":        "Os códigos de login são guardados só como hash e apagados em até 1 dia. O histórico de preços pertence aos decks, não à conta.",
	})
}

// DELETE /api/me {confirm: "<e-mail>"}: apaga a conta e tudo ligado a ela.
func (a *App) handleDeleteAccount(w http.ResponseWriter, r *http.Request, u user) {
	var in struct {
		Confirm string `json:"confirm"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if e, _ := normEmail(in.Confirm); e != u.Email {
		writeErr(w, http.StatusBadRequest, "Digite o seu e-mail para confirmar.")
		return
	}
	ctx := r.Context()
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		serverErr(w, err)
		return
	}
	defer tx.Rollback()
	for _, q := range []string{
		`DELETE FROM user_decks WHERE user_id = ?`,
		`DELETE FROM alerts WHERE user_id = ?`,
		`DELETE FROM shares WHERE user_id = ?`,
		`DELETE FROM sessions WHERE user_id = ?`,
		`DELETE FROM users WHERE id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, q, u.ID); err != nil {
			serverErr(w, err)
			return
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM login_codes WHERE email = ?`, u.Email); err != nil {
		serverErr(w, err)
		return
	}
	if err := tx.Commit(); err != nil {
		serverErr(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---- painel de uso (admin) ----

type dayCount struct {
	Day   string `json:"day"`
	Count int64  `json:"count"`
}

// GET /api/admin/stats
func (a *App) handleAdminStats(w http.ResponseWriter, r *http.Request, u user) {
	if !a.isAdmin(u) {
		writeErr(w, http.StatusForbidden, "Só para administradores.")
		return
	}
	ctx := r.Context()
	now := a.now()
	since30 := now.AddDate(0, 0, -30)
	count := func(q string, args ...any) int64 {
		var n int64
		a.db.QueryRowContext(ctx, q, args...).Scan(&n)
		return n
	}
	totals := map[string]int64{
		"users":           count(`SELECT COUNT(*) FROM users`),
		"users_new_7d":    count(`SELECT COUNT(*) FROM users WHERE created_at >= ?`, now.AddDate(0, 0, -7).Unix()),
		"users_new_30d":   count(`SELECT COUNT(*) FROM users WHERE created_at >= ?`, since30.Unix()),
		"users_logged_in": count(`SELECT COUNT(DISTINCT user_id) FROM sessions WHERE expires_at > ?`, now.Unix()),
		"users_with_deck": count(`SELECT COUNT(DISTINCT user_id) FROM user_decks`),
		"decks_tracked":   count(`SELECT COUNT(DISTINCT deck_id) FROM user_decks`),
		"follows":         count(`SELECT COUNT(*) FROM user_decks`),
		"snapshots":       count(`SELECT COUNT(*) FROM snapshots`),
		"snapshots_today": count(`SELECT COUNT(*) FROM snapshots WHERE day = ?`, a.today()),
		"cards":           count(`SELECT COUNT(*) FROM cards`),
		"alerts":          count(`SELECT COUNT(*) FROM alerts`),
		"shares":          count(`SELECT COUNT(*) FROM shares`),
	}

	// Séries dos últimos 30 dias, com zero nos dias sem movimento.
	days := make([]string, 31)
	for i := range days {
		days[i] = since30.AddDate(0, 0, i).In(a.loc).Format("2006-01-02")
	}
	series := func(m map[string]int64) []dayCount {
		out := make([]dayCount, len(days))
		for i, d := range days {
			out[i] = dayCount{d, m[d]}
		}
		return out
	}
	signups := map[string]int64{}
	if rows, err := a.db.QueryContext(ctx, `SELECT created_at FROM users WHERE created_at >= ?`, since30.Unix()); err == nil {
		for rows.Next() {
			var t int64
			if rows.Scan(&t) == nil {
				signups[time.Unix(t, 0).In(a.loc).Format("2006-01-02")]++
			}
		}
		rows.Close()
	}
	snaps := map[string]int64{}
	if rows, err := a.db.QueryContext(ctx, `SELECT day, COUNT(*) FROM snapshots WHERE day >= ? GROUP BY day`, days[0]); err == nil {
		for rows.Next() {
			var d string
			var n int64
			if rows.Scan(&d, &n) == nil {
				snaps[d] = n
			}
		}
		rows.Close()
	}

	type topDeck struct {
		ID        int64  `json:"id"`
		Name      string `json:"name"`
		URL       string `json:"url"`
		Followers int64  `json:"followers"`
		Snapshots int64  `json:"snapshots"`
		LastDay   string `json:"last_day"`
	}
	top := []topDeck{}
	if rows, err := a.db.QueryContext(ctx, `
		SELECT d.id, d.name, COUNT(ud.user_id),
		  (SELECT COUNT(*) FROM snapshots s WHERE s.deck_id = d.id),
		  COALESCE((SELECT MAX(day) FROM snapshots s WHERE s.deck_id = d.id), '')
		FROM decks d JOIN user_decks ud ON ud.deck_id = d.id
		GROUP BY d.id ORDER BY COUNT(ud.user_id) DESC, d.id LIMIT 10`); err == nil {
		for rows.Next() {
			var t topDeck
			if rows.Scan(&t.ID, &t.Name, &t.Followers, &t.Snapshots, &t.LastDay) == nil {
				t.URL = deckURL(t.ID)
				top = append(top, t)
			}
		}
		rows.Close()
	}

	type recentUser struct {
		Email   string `json:"email"`
		Created string `json:"created_at"`
		Decks   int64  `json:"decks"`
	}
	recent := []recentUser{}
	if rows, err := a.db.QueryContext(ctx, `
		SELECT u.email, u.created_at, (SELECT COUNT(*) FROM user_decks ud WHERE ud.user_id = u.id)
		FROM users u ORDER BY u.created_at DESC LIMIT 20`); err == nil {
		for rows.Next() {
			var ru recentUser
			var c int64
			if rows.Scan(&ru.Email, &c, &ru.Decks) == nil {
				ru.Created = time.Unix(c, 0).In(a.loc).Format("2006-01-02 15:04")
				recent = append(recent, ru)
			}
		}
		rows.Close()
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"totals":            totals,
		"signups_per_day":   series(signups),
		"snapshots_per_day": series(snaps),
		"top_decks":         top,
		"recent_users":      recent,
		"generated_at":      fmt.Sprint(now.In(a.loc).Format("2006-01-02 15:04")),
	})
}
