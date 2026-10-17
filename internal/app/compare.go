package app

import (
	"context"
	"database/sql"
	"net/http"
	"strconv"
	"strings"
)

type cardInfo struct {
	Type string
	Cmc  *int64
	Mana string
	Has  bool // veio algum dado
}

// Grupos que não são tipo de carta.
var notTypes = map[string]bool{"maybeboard": true, "sideboard": true, "comandante": true, "commander": true}

func cardInfoFrom(c snapCard) (cardInfo, bool) {
	inf := cardInfo{}
	t := trimName(c.Type)
	if len([]rune(t)) > 40 || notTypes[strings.ToLower(t)] {
		t = ""
	}
	inf.Type = t
	if c.Cmc != nil && *c.Cmc >= 0 && *c.Cmc <= 30 {
		inf.Cmc = c.Cmc
	}
	if c.Mana != nil && len(*c.Mana) <= 60 {
		inf.Mana = *c.Mana
	}
	inf.Has = inf.Type != "" || inf.Cmc != nil
	return inf, inf.Has
}

// mergeInfo junta linhas da mesma carta (ex.: comandante + cópia no maybeboard): o que tiver tipo ganha.
func mergeInfo(old, new cardInfo) cardInfo {
	if !old.Has {
		return new
	}
	if old.Type == "" {
		old.Type = new.Type
	}
	if old.Cmc == nil {
		old.Cmc, old.Mana = new.Cmc, new.Mana
	}
	return old
}

func upsertCardInfo(ctx context.Context, tx *sql.Tx, infos map[string]cardInfo, now int64) error {
	keys := make([]string, 0, len(infos))
	for k := range infos {
		keys = append(keys, k)
	}
	for i := 0; i < len(keys); i += 150 {
		chunk := keys[i:min(i+150, len(keys))]
		args := make([]any, 0, len(chunk)*5)
		for _, k := range chunk {
			inf := infos[k]
			args = append(args, k, inf.Type, inf.Cmc, inf.Mana, now)
		}
		// Não apaga o que já se sabe com um valor vazio (ex.: carta vista só no maybeboard).
		q := `INSERT INTO card_info (card_key, type, cmc, mana, updated_at) VALUES ` + placeholders(len(chunk), 5) + `
			ON CONFLICT (card_key) DO UPDATE SET
			  type = CASE WHEN excluded.type <> '' THEN excluded.type ELSE card_info.type END,
			  cmc  = COALESCE(excluded.cmc, card_info.cmc),
			  mana = CASE WHEN excluded.cmc IS NOT NULL THEN excluded.mana ELSE card_info.mana END,
			  updated_at = excluded.updated_at`
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return err
		}
	}
	return nil
}

type compareDeck struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Day  string `json:"day"`
	Avg  *int64 `json:"total_avg"`
}

type compareCard struct {
	Key   string `json:"key"`
	Name  string `json:"name"`
	Board string `json:"board"`
	Type  string `json:"type"`
	Cmc   *int64 `json:"cmc"`
	Mana  string `json:"mana"`
	QtyA  int64  `json:"qty_a"`
	QtyB  int64  `json:"qty_b"`
	Unit  *int64 `json:"unit_avg"` // preço médio de 1 cópia
}

// GET /api/compare?a=ID&b=ID -> cartas dos últimos snapshots dos dois decks (deck e sideboard; maybeboard fica de fora).
func (a *App) handleCompare(w http.ResponseWriter, r *http.Request, u user) {
	ctx := r.Context()
	ids := [2]int64{}
	for i, p := range []string{"a", "b"} {
		id, err := strconv.ParseInt(r.URL.Query().Get(p), 10, 64)
		if err != nil || id <= 0 {
			writeErr(w, http.StatusBadRequest, "Escolha dois decks.")
			return
		}
		ok, err := a.isTracking(ctx, u.ID, id)
		if err != nil {
			serverErr(w, err)
			return
		}
		if !ok {
			writeErr(w, http.StatusNotFound, "Você não acompanha o deck "+strconv.FormatInt(id, 10)+".")
			return
		}
		ids[i] = id
	}
	if ids[0] == ids[1] {
		writeErr(w, http.StatusBadRequest, "Escolha dois decks diferentes.")
		return
	}

	decks := [2]compareDeck{}
	snaps := [2]sql.NullInt64{}
	for i, id := range ids {
		d := compareDeck{ID: id}
		var day sql.NullString
		var avg sql.NullInt64
		err := a.db.QueryRowContext(ctx, `
			SELECT d.name, s.id, s.day, s.total_avg FROM decks d
			LEFT JOIN snapshots s ON s.id = (SELECT id FROM snapshots WHERE deck_id = d.id ORDER BY day DESC LIMIT 1)
			WHERE d.id = ?`, id).Scan(&d.Name, &snaps[i], &day, &avg)
		if err != nil {
			serverErr(w, err)
			return
		}
		d.Day, d.Avg = day.String, nullPtr(avg)
		decks[i] = d
	}
	for i, s := range snaps {
		if !s.Valid {
			writeErr(w, http.StatusUnprocessableEntity, "O deck \""+decks[i].Name+"\" ainda não tem preços coletados.")
			return
		}
	}

	rows, err := a.db.QueryContext(ctx, `
		SELECT sc.snapshot_id, sc.board, sc.card_key, c.name, COALESCE(ci.type, ''), ci.cmc, COALESCE(ci.mana, ''), sc.qty, sc.price_avg
		FROM snapshot_cards sc
		JOIN cards c ON c.card_key = sc.card_key
		LEFT JOIN card_info ci ON ci.card_key = sc.card_key
		WHERE sc.snapshot_id IN (?, ?) AND sc.board IN ('main', 'side')`, snaps[0].Int64, snaps[1].Int64)
	if err != nil {
		serverErr(w, err)
		return
	}
	defer rows.Close()
	type k struct{ board, key string }
	byKey := map[k]*compareCard{}
	order := []k{}
	for rows.Next() {
		var snap, qty int64
		var c compareCard
		var cmc, avg sql.NullInt64
		if err := rows.Scan(&snap, &c.Board, &c.Key, &c.Name, &c.Type, &cmc, &c.Mana, &qty, &avg); err != nil {
			serverErr(w, err)
			return
		}
		kk := k{c.Board, c.Key}
		cur, ok := byKey[kk]
		if !ok {
			c.Cmc = nullPtr(cmc)
			cur = &c
			byKey[kk] = cur
			order = append(order, kk)
		}
		if snap == snaps[0].Int64 {
			cur.QtyA += qty
		} else {
			cur.QtyB += qty
		}
		// O preço salvo é o da linha (todas as cópias): guarda o de 1 cópia.
		if avg.Valid && qty > 0 && cur.Unit == nil {
			v := avg.Int64 / qty
			cur.Unit = &v
		}
	}
	if err := rows.Err(); err != nil {
		serverErr(w, err)
		return
	}
	out := make([]compareCard, 0, len(order))
	for _, kk := range order {
		out = append(out, *byKey[kk])
	}
	writeJSON(w, http.StatusOK, map[string]any{"a": decks[0], "b": decks[1], "cards": out})
}
