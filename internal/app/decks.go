package app

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"
)

type deckHandler func(http.ResponseWriter, *http.Request, user, int64)

func (a *App) isTracking(ctx context.Context, uid, deckID int64) (bool, error) {
	var n int
	err := a.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM user_decks WHERE user_id = ? AND deck_id = ?`, uid, deckID).Scan(&n)
	return n > 0, err
}

// tracked garante que o usuário acompanha o deck da URL.
func (a *App) tracked(next deckHandler) authedHandler {
	return func(w http.ResponseWriter, r *http.Request, u user) {
		id, ok := pathDeckID(r)
		if !ok {
			writeErr(w, http.StatusBadRequest, "ID de deck inválido.")
			return
		}
		ok, err := a.isTracking(r.Context(), u.ID, id)
		if err != nil {
			serverErr(w, err)
			return
		}
		if !ok {
			writeErr(w, http.StatusNotFound, "Você não acompanha este deck.")
			return
		}
		next(w, r, u, id)
	}
}

type deckSummary struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	LastDay  string `json:"last_day,omitempty"`
	TotalMin *int64 `json:"total_min"`
	TotalAvg *int64 `json:"total_avg"`
	TotalMax *int64 `json:"total_max"`
	PrevMin  *int64 `json:"prev_min"`
	// SnapshotToday indica se o preço de hoje já foi salvo (a extensão usa para a coleta automática).
	SnapshotToday bool `json:"snapshot_today"`
}

// GET /api/decks
func (a *App) handleListDecks(w http.ResponseWriter, r *http.Request, u user) {
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT d.id, d.name, s.day, s.total_min, s.total_avg, s.total_max,
		  (SELECT p.total_min FROM snapshots p WHERE p.deck_id = d.id AND p.day < s.day ORDER BY p.day DESC LIMIT 1)
		FROM user_decks ud
		JOIN decks d ON d.id = ud.deck_id
		LEFT JOIN snapshots s ON s.id = (SELECT x.id FROM snapshots x WHERE x.deck_id = d.id ORDER BY x.day DESC LIMIT 1)
		WHERE ud.user_id = ?
		ORDER BY ud.created_at DESC`, u.ID)
	if err != nil {
		serverErr(w, err)
		return
	}
	defer rows.Close()
	out := []deckSummary{}
	today := a.today()
	for rows.Next() {
		var d deckSummary
		var day sql.NullString
		var mn, av, mx, prev sql.NullInt64
		if err := rows.Scan(&d.ID, &d.Name, &day, &mn, &av, &mx, &prev); err != nil {
			serverErr(w, err)
			return
		}
		d.URL, d.LastDay, d.SnapshotToday = deckURL(d.ID), day.String, day.String == today
		d.TotalMin, d.TotalAvg, d.TotalMax, d.PrevMin = nullPtr(mn), nullPtr(av), nullPtr(mx), nullPtr(prev)
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// POST /api/decks {deck: "id ou URL"}
func (a *App) handleAddDeck(w http.ResponseWriter, r *http.Request, u user) {
	var in struct {
		Deck string `json:"deck"`
		Name string `json:"name"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	id, ok := parseDeckRef(in.Deck)
	if !ok {
		writeErr(w, http.StatusBadRequest, "Informe o ID ou a URL do deck na LigaMagic.")
		return
	}
	if err := a.addDeck(r.Context(), u.ID, id, trimName(in.Name)); err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id, "url": deckURL(id)})
}

func (a *App) addDeck(ctx context.Context, uid, id int64, name string) error {
	now := a.now().Unix()
	if _, err := a.db.ExecContext(ctx,
		`INSERT INTO decks (id, name, created_at) VALUES (?, ?, ?) ON CONFLICT (id) DO NOTHING`, id, name, now); err != nil {
		return err
	}
	_, err := a.db.ExecContext(ctx,
		`INSERT INTO user_decks (user_id, deck_id, created_at) VALUES (?, ?, ?) ON CONFLICT DO NOTHING`, uid, id, now)
	return err
}

// DELETE /api/decks/{id}. O histórico do deck continua salvo para quem mais acompanha.
func (a *App) handleRemoveDeck(w http.ResponseWriter, r *http.Request, u user) {
	id, ok := pathDeckID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "ID de deck inválido.")
		return
	}
	if err := a.untrack(r.Context(), a.db, u.ID, id); err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// untrack tira o deck da conta junto com o alerta e o link público dele.
func (a *App) untrack(ctx context.Context, db execer, uid, deckID int64) error {
	for _, q := range []string{
		`DELETE FROM user_decks WHERE user_id = ? AND deck_id = ?`,
		`DELETE FROM alerts WHERE user_id = ? AND deck_id = ?`,
		`DELETE FROM shares WHERE user_id = ? AND deck_id = ?`,
	} {
		if _, err := db.ExecContext(ctx, q, uid, deckID); err != nil {
			return err
		}
	}
	return nil
}

// GET /api/decks/{id}/status -> usado pela extensão
func (a *App) handleDeckStatus(w http.ResponseWriter, r *http.Request, u user) {
	id, ok := pathDeckID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "ID de deck inválido.")
		return
	}
	tracking, err := a.isTracking(r.Context(), u.ID, id)
	if err != nil {
		serverErr(w, err)
		return
	}
	var n int
	if err := a.db.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM snapshots WHERE deck_id = ? AND day = ?`, id, a.today()).Scan(&n); err != nil {
		serverErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tracked": tracking, "snapshot_today": n > 0, "today": a.today()})
}

type snapCard struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Board string `json:"board"`
	Qty   int    `json:"qty"`
	Min   *int64 `json:"min"`
	Avg   *int64 `json:"avg"`
	Max   *int64 `json:"max"`
}

type snapIn struct {
	Name   string `json:"name"`
	Totals struct {
		Min *int64 `json:"min"`
		Avg *int64 `json:"avg"`
		Max *int64 `json:"max"`
	} `json:"totals"`
	Cards []snapCard `json:"cards"`
}

const maxCents = 100_000_000 // R$ 1 milhão

func validCents(p *int64) bool { return p == nil || (*p >= 0 && *p <= maxCents) }

func trimName(s string) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > 200 {
		s = string(r[:200])
	}
	return s
}

func addPtr(a, b *int64) *int64 {
	if a == nil {
		return b
	}
	if b == nil {
		return a
	}
	v := *a + *b
	return &v
}

// POST /api/decks/{id}/snapshot (enviado pela extensão)
func (a *App) handleSnapshot(w http.ResponseWriter, r *http.Request, u user) {
	id, ok := pathDeckID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "ID de deck inválido.")
		return
	}
	ctx := r.Context()
	if ok, err := a.isTracking(ctx, u.ID, id); err != nil {
		serverErr(w, err)
		return
	} else if !ok {
		writeErr(w, http.StatusNotFound, "Você não acompanha este deck.")
		return
	}
	var in snapIn
	if !readJSON(w, r, &in) {
		return
	}
	if len(in.Cards) == 0 || len(in.Cards) > 1000 {
		writeErr(w, http.StatusUnprocessableEntity, "Lista de cartas vazia ou grande demais.")
		return
	}
	t := in.Totals
	if !validCents(t.Min) || !validCents(t.Avg) || !validCents(t.Max) {
		writeErr(w, http.StatusUnprocessableEntity, "Total inválido.")
		return
	}
	if t.Min != nil && t.Avg != nil && t.Max != nil && !(*t.Min <= *t.Avg && *t.Avg <= *t.Max) {
		writeErr(w, http.StatusUnprocessableEntity, "Totais fora de ordem (mín ≤ médio ≤ máx).")
		return
	}

	// Junta linhas repetidas (mesma carta em edições diferentes).
	type k struct{ board, key string }
	merged := map[k]*snapCard{}
	order := []k{}
	names := map[string]string{}
	for _, c := range in.Cards {
		c.Key, c.Name = strings.TrimSpace(c.Key), trimName(c.Name)
		if c.Key == "" || len(c.Key) > 120 || c.Name == "" {
			writeErr(w, http.StatusUnprocessableEntity, "Carta sem identificação.")
			return
		}
		if c.Board != "main" && c.Board != "side" && c.Board != "maybe" {
			writeErr(w, http.StatusUnprocessableEntity, "Board inválido: "+c.Board)
			return
		}
		if c.Qty < 1 || c.Qty > 999 || !validCents(c.Min) || !validCents(c.Avg) || !validCents(c.Max) {
			writeErr(w, http.StatusUnprocessableEntity, "Valores inválidos na carta "+c.Name)
			return
		}
		names[c.Key] = c.Name
		kk := k{c.Board, c.Key}
		if m, ok := merged[kk]; ok {
			m.Qty += c.Qty
			m.Min, m.Avg, m.Max = addPtr(m.Min, c.Min), addPtr(m.Avg, c.Avg), addPtr(m.Max, c.Max)
			continue
		}
		cc := c
		merged[kk] = &cc
		order = append(order, kk)
	}

	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		serverErr(w, err)
		return
	}
	defer tx.Rollback()

	if name := trimName(in.Name); name != "" {
		if _, err := tx.ExecContext(ctx, `UPDATE decks SET name = ? WHERE id = ?`, name, id); err != nil {
			serverErr(w, err)
			return
		}
	}
	var snapID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO snapshots (deck_id, day, captured_at, total_min, total_avg, total_max)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (deck_id, day) DO UPDATE SET
		  captured_at = excluded.captured_at, total_min = excluded.total_min,
		  total_avg = excluded.total_avg, total_max = excluded.total_max
		RETURNING id`, id, a.today(), a.now().Unix(), t.Min, t.Avg, t.Max).Scan(&snapID)
	if err != nil {
		serverErr(w, err)
		return
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM snapshot_cards WHERE snapshot_id = ?`, snapID); err != nil {
		serverErr(w, err)
		return
	}

	// Inserts em lote para reduzir idas e voltas ao Turso.
	keys := make([]string, 0, len(names))
	for key := range names {
		keys = append(keys, key)
	}
	for i := 0; i < len(keys); i += 200 {
		chunk := keys[i:min(i+200, len(keys))]
		args := make([]any, 0, len(chunk)*2)
		for _, key := range chunk {
			args = append(args, key, names[key])
		}
		q := `INSERT INTO cards (card_key, name) VALUES ` + placeholders(len(chunk), 2) +
			` ON CONFLICT (card_key) DO UPDATE SET name = excluded.name`
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			serverErr(w, err)
			return
		}
	}
	for i := 0; i < len(order); i += 100 {
		chunk := order[i:min(i+100, len(order))]
		args := make([]any, 0, len(chunk)*7)
		for _, kk := range chunk {
			c := merged[kk]
			args = append(args, snapID, c.Board, c.Key, c.Qty, c.Min, c.Avg, c.Max)
		}
		q := `INSERT INTO snapshot_cards (snapshot_id, board, card_key, qty, price_min, price_avg, price_max) VALUES ` + placeholders(len(chunk), 7)
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			serverErr(w, err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		serverErr(w, err)
		return
	}
	a.checkAlerts(ctx, id)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "day": a.today(), "cards": len(order)})
}

func placeholders(rows, cols int) string {
	row := "(" + strings.TrimSuffix(strings.Repeat("?, ", cols), ", ") + ")"
	return strings.TrimSuffix(strings.Repeat(row+", ", rows), ", ")
}

type point struct {
	Day string `json:"day"`
	Qty *int64 `json:"qty,omitempty"`
	Min *int64 `json:"min"`
	Avg *int64 `json:"avg"`
	Max *int64 `json:"max"`
}

// GET /api/decks/{id}/history
func (a *App) handleDeckHistory(w http.ResponseWriter, r *http.Request, u user, id int64) {
	var name string
	a.db.QueryRowContext(r.Context(), `SELECT name FROM decks WHERE id = ?`, id).Scan(&name)
	rows, err := a.db.QueryContext(r.Context(),
		`SELECT day, total_min, total_avg, total_max FROM snapshots WHERE deck_id = ? ORDER BY day`, id)
	if err != nil {
		serverErr(w, err)
		return
	}
	defer rows.Close()
	pts := []point{}
	for rows.Next() {
		var p point
		var mn, av, mx sql.NullInt64
		if err := rows.Scan(&p.Day, &mn, &av, &mx); err != nil {
			serverErr(w, err)
			return
		}
		p.Min, p.Avg, p.Max = nullPtr(mn), nullPtr(av), nullPtr(mx)
		pts = append(pts, p)
	}
	out := map[string]any{"id": id, "name": name, "url": deckURL(id), "points": pts}
	if u.ID != 0 { // só o dono vê o alerta e o link público; o link público não expõe nada da conta
		var pct sql.NullInt64
		var token sql.NullString
		a.db.QueryRowContext(r.Context(), `SELECT pct FROM alerts WHERE user_id = ? AND deck_id = ?`, u.ID, id).Scan(&pct)
		a.db.QueryRowContext(r.Context(), `SELECT token FROM shares WHERE user_id = ? AND deck_id = ?`, u.ID, id).Scan(&token)
		out["alert_pct"] = nullPtr(pct)
		out["share_url"] = nil
		if token.Valid {
			out["share_url"] = a.shareURL(token.String)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

type cardRow struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	Board   string `json:"board"`
	Qty     int    `json:"qty"`
	Min     *int64 `json:"min"`
	Avg     *int64 `json:"avg"`
	Max     *int64 `json:"max"`
	PrevMin *int64 `json:"prev_min"`
}

// GET /api/decks/{id}/cards -> cartas do snapshot mais recente + preço mínimo do anterior
func (a *App) handleDeckCards(w http.ResponseWriter, r *http.Request, u user, id int64) {
	ctx := r.Context()
	var latestID int64
	var day string
	err := a.db.QueryRowContext(ctx, `SELECT id, day FROM snapshots WHERE deck_id = ? ORDER BY day DESC LIMIT 1`, id).Scan(&latestID, &day)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusOK, map[string]any{"day": nil, "prev_day": nil, "cards": []cardRow{}})
		return
	}
	if err != nil {
		serverErr(w, err)
		return
	}
	var prevID sql.NullInt64
	var prevDay sql.NullString
	a.db.QueryRowContext(ctx, `SELECT id, day FROM snapshots WHERE deck_id = ? AND day < ? ORDER BY day DESC LIMIT 1`, id, day).Scan(&prevID, &prevDay)

	rows, err := a.db.QueryContext(ctx, `
		SELECT sc.card_key, c.name, sc.board, sc.qty, sc.price_min, sc.price_avg, sc.price_max, p.price_min
		FROM snapshot_cards sc
		JOIN cards c ON c.card_key = sc.card_key
		LEFT JOIN snapshot_cards p ON p.snapshot_id = ? AND p.board = sc.board AND p.card_key = sc.card_key
		WHERE sc.snapshot_id = ?
		ORDER BY CASE sc.board WHEN 'main' THEN 0 WHEN 'side' THEN 1 ELSE 2 END, sc.price_min DESC, c.name`,
		prevID, latestID)
	if err != nil {
		serverErr(w, err)
		return
	}
	defer rows.Close()
	out := []cardRow{}
	for rows.Next() {
		var c cardRow
		var mn, av, mx, pv sql.NullInt64
		if err := rows.Scan(&c.Key, &c.Name, &c.Board, &c.Qty, &mn, &av, &mx, &pv); err != nil {
			serverErr(w, err)
			return
		}
		c.Min, c.Avg, c.Max, c.PrevMin = nullPtr(mn), nullPtr(av), nullPtr(mx), nullPtr(pv)
		out = append(out, c)
	}
	writeJSON(w, http.StatusOK, map[string]any{"day": day, "prev_day": prevDay.String, "cards": out})
}

// GET /api/decks/{id}/card-history?key=...
func (a *App) handleCardHistory(w http.ResponseWriter, r *http.Request, u user, id int64) {
	key := r.URL.Query().Get("key")
	if key == "" {
		writeErr(w, http.StatusBadRequest, "Informe a carta.")
		return
	}
	var name string
	a.db.QueryRowContext(r.Context(), `SELECT name FROM cards WHERE card_key = ?`, key).Scan(&name)
	rows, err := a.db.QueryContext(r.Context(), `
		SELECT s.day, SUM(sc.qty), SUM(sc.price_min), SUM(sc.price_avg), SUM(sc.price_max)
		FROM snapshot_cards sc JOIN snapshots s ON s.id = sc.snapshot_id
		WHERE s.deck_id = ? AND sc.card_key = ?
		GROUP BY s.day ORDER BY s.day`, id, key)
	if err != nil {
		serverErr(w, err)
		return
	}
	defer rows.Close()
	pts := []point{}
	for rows.Next() {
		var p point
		var q, mn, av, mx sql.NullInt64
		if err := rows.Scan(&p.Day, &q, &mn, &av, &mx); err != nil {
			serverErr(w, err)
			return
		}
		p.Qty, p.Min, p.Avg, p.Max = nullPtr(q), nullPtr(mn), nullPtr(av), nullPtr(mx)
		pts = append(pts, p)
	}
	writeJSON(w, http.StatusOK, map[string]any{"key": key, "name": name, "points": pts})
}

// POST /api/transfer {email, mode: "move"|"copy", deck_ids?: [..]}
func (a *App) handleTransfer(w http.ResponseWriter, r *http.Request, u user) {
	var in struct {
		Email   string  `json:"email"`
		Mode    string  `json:"mode"`
		DeckIDs []int64 `json:"deck_ids"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	target, ok := normEmail(in.Email)
	if !ok {
		writeErr(w, http.StatusBadRequest, "E-mail inválido.")
		return
	}
	if target == u.Email {
		writeErr(w, http.StatusBadRequest, "Esse já é o seu e-mail.")
		return
	}
	if in.Mode != "move" && in.Mode != "copy" {
		writeErr(w, http.StatusBadRequest, "Escolha mover ou copiar.")
		return
	}
	ctx := r.Context()

	rows, err := a.db.QueryContext(ctx, `SELECT deck_id FROM user_decks WHERE user_id = ?`, u.ID)
	if err != nil {
		serverErr(w, err)
		return
	}
	mine := map[int64]bool{}
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		mine[id] = true
	}
	rows.Close()

	ids := in.DeckIDs
	if len(ids) == 0 {
		for id := range mine {
			ids = append(ids, id)
		}
	}
	for _, id := range ids {
		if !mine[id] {
			writeErr(w, http.StatusBadRequest, fmt.Sprintf("Você não acompanha o deck %d.", id))
			return
		}
	}
	if len(ids) == 0 {
		writeErr(w, http.StatusBadRequest, "Você não tem decks para transferir.")
		return
	}

	targetID, err := a.upsertUser(r, target)
	if err != nil {
		serverErr(w, err)
		return
	}
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		serverErr(w, err)
		return
	}
	defer tx.Rollback()
	now := a.now().Unix()
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO user_decks (user_id, deck_id, created_at) VALUES (?, ?, ?) ON CONFLICT DO NOTHING`, targetID, id, now); err != nil {
			serverErr(w, err)
			return
		}
		if in.Mode == "move" {
			if err := a.untrack(ctx, tx, u.ID, id); err != nil {
				serverErr(w, err)
				return
			}
		}
	}
	if err := tx.Commit(); err != nil {
		serverErr(w, err)
		return
	}

	verb := "copiou"
	if in.Mode == "move" {
		verb = "transferiu"
	}
	text := fmt.Sprintf("%s %s %d deck(s) para a sua conta no MTG Tracker.\n\nEntre em %s com este e-mail para ver.", u.Email, verb, len(ids), a.baseURL)
	html := fmt.Sprintf(`<p><b>%s</b> %s %d deck(s) para a sua conta no MTG Tracker.</p><p><a href="%s">Entre com este e-mail</a> para ver.</p>`, u.Email, verb, len(ids), a.baseURL)
	if err := a.mail.Send(ctx, target, "Decks compartilhados com você", text, html); err != nil {
		// A transferência já foi feita; o aviso por e-mail é só cortesia.
		fmt.Printf("aviso de transferência falhou: %v\n", err)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "decks": len(ids), "mode": in.Mode})
}
