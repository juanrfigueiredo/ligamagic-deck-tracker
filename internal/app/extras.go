package app

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/juanrfigueiredo/ligamagic-deck-tracker/extension"
)

// ---- alertas ----

// PUT /api/decks/{id}/alert {pct: 1..100 | null}
func (a *App) handleSetAlert(w http.ResponseWriter, r *http.Request, u user, id int64) {
	var in struct {
		Pct *int `json:"pct"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	ctx := r.Context()
	if in.Pct == nil {
		if _, err := a.db.ExecContext(ctx, `DELETE FROM alerts WHERE user_id = ? AND deck_id = ?`, u.ID, id); err != nil {
			serverErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"pct": nil})
		return
	}
	if *in.Pct < 1 || *in.Pct > 100 {
		writeErr(w, http.StatusBadRequest, "Escolha uma variação entre 1% e 100%.")
		return
	}
	if _, err := a.db.ExecContext(ctx,
		`INSERT INTO alerts (user_id, deck_id, pct) VALUES (?, ?, ?)
		 ON CONFLICT (user_id, deck_id) DO UPDATE SET pct = excluded.pct`, u.ID, id, *in.Pct); err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pct": *in.Pct})
}

type mover struct {
	Name     string
	Prev     int64
	Cur      int64
	Variance float64
}

// checkAlerts roda depois de cada snapshot. Compara o preço mínimo de hoje com o do snapshot anterior
// e manda no máximo 1 e-mail por dia para cada usuário cujo limite foi atingido.
func (a *App) checkAlerts(ctx context.Context, deckID int64) {
	today := a.today()
	var name string
	var cur, prev sql.NullInt64
	var curID, prevID int64
	var prevDay string
	if err := a.db.QueryRowContext(ctx,
		`SELECT s.id, s.total_min, d.name FROM snapshots s JOIN decks d ON d.id = s.deck_id WHERE s.deck_id = ? AND s.day = ?`,
		deckID, today).Scan(&curID, &cur, &name); err != nil {
		return
	}
	if err := a.db.QueryRowContext(ctx,
		`SELECT id, total_min, day FROM snapshots WHERE deck_id = ? AND day < ? ORDER BY day DESC LIMIT 1`,
		deckID, today).Scan(&prevID, &prev, &prevDay); err != nil {
		return
	}
	if !cur.Valid || !prev.Valid || prev.Int64 == 0 {
		return
	}
	change := float64(cur.Int64-prev.Int64) * 100 / float64(prev.Int64)

	rows, err := a.db.QueryContext(ctx, `
		SELECT a.user_id, u.email, a.pct FROM alerts a
		JOIN users u ON u.id = a.user_id
		JOIN user_decks ud ON ud.user_id = a.user_id AND ud.deck_id = a.deck_id
		WHERE a.deck_id = ? AND (a.last_sent_day IS NULL OR a.last_sent_day <> ?)`, deckID, today)
	if err != nil {
		log.Printf("alertas: %v", err)
		return
	}
	type target struct {
		id    int64
		email string
	}
	var targets []target
	for rows.Next() {
		var t target
		var pct int
		if rows.Scan(&t.id, &t.email, &pct) == nil && math.Abs(change) >= float64(pct) {
			targets = append(targets, t)
		}
	}
	rows.Close()
	if len(targets) == 0 {
		return
	}

	movers := a.topMovers(ctx, curID, prevID, 3)
	if name == "" {
		name = "Deck " + strconv.FormatInt(deckID, 10)
	}
	dir := "subiu"
	if change < 0 {
		dir = "caiu"
	}
	link := fmt.Sprintf("%s/#/deck/%d", a.baseURL, deckID)
	subject := fmt.Sprintf("%s %s %s", name, dir, fmtPct(change))
	text := fmt.Sprintf("O preço mínimo de %s %s %s: de %s (%s) para %s (hoje).\n", name, dir, fmtPct(change), fmtBRL(prev.Int64), fmtDay(prevDay), fmtBRL(cur.Int64))
	htm := fmt.Sprintf("<p>O preço mínimo de <b>%s</b> %s <b>%s</b>: de %s (%s) para <b>%s</b> (hoje).</p>",
		html.EscapeString(name), dir, fmtPct(change), fmtBRL(prev.Int64), fmtDay(prevDay), fmtBRL(cur.Int64))
	if len(movers) > 0 {
		text += "\nCartas que mais mudaram:\n"
		htm += "<p>Cartas que mais mudaram:</p><ul>"
		for _, m := range movers {
			text += fmt.Sprintf("- %s: %s → %s (%s)\n", m.Name, fmtBRL(m.Prev), fmtBRL(m.Cur), fmtPct(m.Variance))
			htm += fmt.Sprintf("<li>%s: %s → %s (%s)</li>", html.EscapeString(m.Name), fmtBRL(m.Prev), fmtBRL(m.Cur), fmtPct(m.Variance))
		}
		htm += "</ul>"
	}
	text += "\nVer o histórico: " + link + "\nPara parar de receber, desligue o alerta na página do deck."
	htm += fmt.Sprintf(`<p><a href="%s">Ver o histórico</a></p><p style="color:#666;font-size:12px">Para parar de receber, desligue o alerta na página do deck.</p>`, link)

	for _, t := range targets {
		if err := a.mail.Send(ctx, t.email, subject, text, htm); err != nil {
			log.Printf("alerta para %s: %v", t.email, err)
			continue
		}
		a.db.ExecContext(ctx, `UPDATE alerts SET last_sent_day = ? WHERE user_id = ? AND deck_id = ?`, today, t.id, deckID)
	}
}

// topMovers devolve as cartas cujo preço mínimo da linha mais variou entre dois snapshots.
func (a *App) topMovers(ctx context.Context, curID, prevID int64, n int) []mover {
	rows, err := a.db.QueryContext(ctx, `
		SELECT c.name, p.price_min, sc.price_min FROM snapshot_cards sc
		JOIN snapshot_cards p ON p.snapshot_id = ? AND p.board = sc.board AND p.card_key = sc.card_key
		JOIN cards c ON c.card_key = sc.card_key
		WHERE sc.snapshot_id = ? AND sc.price_min IS NOT NULL AND p.price_min > 0`, prevID, curID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []mover
	for rows.Next() {
		var m mover
		if rows.Scan(&m.Name, &m.Prev, &m.Cur) == nil && m.Cur != m.Prev {
			m.Variance = float64(m.Cur-m.Prev) * 100 / float64(m.Prev)
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return math.Abs(out[i].Variance) > math.Abs(out[j].Variance) })
	if len(out) > n {
		out = out[:n]
	}
	return out
}

func fmtBRL(c int64) string {
	reais := strconv.FormatInt(c/100, 10)
	for i := len(reais) - 3; i > 0; i -= 3 {
		reais = reais[:i] + "." + reais[i:]
	}
	return fmt.Sprintf("R$ %s,%02d", reais, c%100)
}

func fmtPct(p float64) string {
	s := strconv.FormatFloat(math.Abs(p), 'f', 1, 64)
	sign := "+"
	if p < 0 {
		sign = "-"
	}
	return sign + strings.Replace(s, ".", ",", 1) + "%"
}

func fmtDay(d string) string {
	if len(d) != 10 {
		return d
	}
	return d[8:] + "/" + d[5:7] + "/" + d[:4]
}

// ---- link público ----

func newShareToken() string {
	b := make([]byte, 12)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func (a *App) shareURL(token string) string { return a.baseURL + "/#/p/" + token }

// POST /api/decks/{id}/share -> cria (ou devolve) o link público do deck
func (a *App) handleShareCreate(w http.ResponseWriter, r *http.Request, u user, id int64) {
	ctx := r.Context()
	if _, err := a.db.ExecContext(ctx,
		`INSERT INTO shares (token, deck_id, user_id, created_at) VALUES (?, ?, ?, ?) ON CONFLICT DO NOTHING`,
		newShareToken(), id, u.ID, a.now().Unix()); err != nil {
		serverErr(w, err)
		return
	}
	var token string
	if err := a.db.QueryRowContext(ctx, `SELECT token FROM shares WHERE user_id = ? AND deck_id = ?`, u.ID, id).Scan(&token); err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "url": a.shareURL(token)})
}

// DELETE /api/decks/{id}/share -> o link antigo para de funcionar
func (a *App) handleShareDelete(w http.ResponseWriter, r *http.Request, u user, id int64) {
	if _, err := a.db.ExecContext(r.Context(), `DELETE FROM shares WHERE user_id = ? AND deck_id = ?`, u.ID, id); err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// public resolve /api/public/{token}/... para o deck compartilhado. Não exige login.
func (a *App) public(next deckHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var id int64
		err := a.db.QueryRowContext(r.Context(), `SELECT deck_id FROM shares WHERE token = ?`, r.PathValue("token")).Scan(&id)
		if err == sql.ErrNoRows {
			writeErr(w, http.StatusNotFound, "Link inválido ou desativado.")
			return
		}
		if err != nil {
			serverErr(w, err)
			return
		}
		next(w, r, user{}, id)
	}
}

// ---- exportar CSV ----

// GET /api/decks/{id}/export.csv: uma linha de total por dia e uma por carta.
// Separador ";" e vírgula decimal, que o Excel em português abre direto.
func (a *App) handleExportCSV(w http.ResponseWriter, r *http.Request, u user, id int64) {
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT s.day, 'TOTAL', '', '', NULL, s.total_min, s.total_avg, s.total_max, 0 FROM snapshots s WHERE s.deck_id = ?
		UNION ALL
		SELECT s.day, 'carta', sc.board, c.name, sc.qty, sc.price_min, sc.price_avg, sc.price_max, 1
		FROM snapshots s JOIN snapshot_cards sc ON sc.snapshot_id = s.id JOIN cards c ON c.card_key = sc.card_key
		WHERE s.deck_id = ?
		ORDER BY 1, 9, 3, 4`, id, id)
	if err != nil {
		serverErr(w, err)
		return
	}
	defer rows.Close()
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="deck-%d.csv"`, id))
	w.Write([]byte("\xef\xbb\xbf")) // BOM: o Excel reconhece UTF-8
	cw := csv.NewWriter(w)
	cw.Comma = ';'
	cw.Write([]string{"data", "tipo", "board", "carta", "qtd", "minimo", "medio", "maximo"})
	cents := func(n sql.NullInt64) string {
		if !n.Valid {
			return ""
		}
		return fmt.Sprintf("%d,%02d", n.Int64/100, n.Int64%100)
	}
	for rows.Next() {
		var day, kind, board, name string
		var qty, mn, av, mx sql.NullInt64
		var order int
		if err := rows.Scan(&day, &kind, &board, &name, &qty, &mn, &av, &mx, &order); err != nil {
			log.Printf("csv: %v", err)
			break
		}
		q := ""
		if qty.Valid {
			q = strconv.FormatInt(qty.Int64, 10)
		}
		cw.Write([]string{day, kind, board, name, q, cents(mn), cents(av), cents(mx)})
	}
	cw.Flush()
}

// ---- extensão ----

var extVersion = func() string {
	var m struct {
		Version string `json:"version"`
	}
	data, _ := extension.Files.ReadFile("manifest.json")
	json.Unmarshal(data, &m)
	return m.Version
}()

// GET /api/extension -> versão atual, para o site avisar quando a instalada estiver velha
func (a *App) handleExtensionInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"version": extVersion})
}
