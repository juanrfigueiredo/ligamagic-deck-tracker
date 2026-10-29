package app

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
)

const maxCompare = 4

type compareDeck struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
	Day  string `json:"day,omitempty"`
}

type compareRow struct {
	Key  string  `json:"key"`
	Name string  `json:"name"`
	Type *string `json:"type"`
	CMC  *int64  `json:"cmc"`
	// Qty[i] = quantidade no deck i (0 = não tem). Mesma ordem de "decks".
	Qty []int `json:"qty"`
}

// GET /api/compare?decks=1,2,3 -> cartas do deck principal (sem side/maybe) do snapshot mais recente de cada deck.
func (a *App) handleCompare(w http.ResponseWriter, r *http.Request, u user) {
	ctx := r.Context()
	var ids []int64
	seen := map[int64]bool{}
	for _, p := range strings.Split(r.URL.Query().Get("decks"), ",") {
		id, err := strconv.ParseInt(strings.TrimSpace(p), 10, 64)
		if err != nil || id <= 0 {
			writeErr(w, http.StatusBadRequest, "Lista de decks inválida.")
			return
		}
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	if len(ids) < 2 || len(ids) > maxCompare {
		writeErr(w, http.StatusBadRequest, "Escolha de 2 a 4 decks.")
		return
	}

	decks := make([]compareDeck, len(ids))
	byKey := map[string]*compareRow{}
	rows := []*compareRow{}
	for i, id := range ids {
		ok, err := a.isTracking(ctx, u.ID, id)
		if err != nil {
			serverErr(w, err)
			return
		}
		if !ok {
			writeErr(w, http.StatusNotFound, "Você não acompanha o deck "+strconv.FormatInt(id, 10)+".")
			return
		}
		d := compareDeck{ID: id, URL: deckURL(id)}
		var day sql.NullString
		var snapID sql.NullInt64
		a.db.QueryRowContext(ctx, `SELECT name FROM decks WHERE id = ?`, id).Scan(&d.Name)
		a.db.QueryRowContext(ctx, `SELECT id, day FROM snapshots WHERE deck_id = ? ORDER BY day DESC LIMIT 1`, id).Scan(&snapID, &day)
		d.Day = day.String
		decks[i] = d
		if !snapID.Valid {
			continue
		}
		rs, err := a.db.QueryContext(ctx, `
			SELECT sc.card_key, c.name, c.type, c.cmc, sc.qty
			FROM snapshot_cards sc JOIN cards c ON c.card_key = sc.card_key
			WHERE sc.snapshot_id = ? AND sc.board = 'main'`, snapID.Int64)
		if err != nil {
			serverErr(w, err)
			return
		}
		for rs.Next() {
			var key, name string
			var typ sql.NullString
			var cmc sql.NullInt64
			var qty int
			if err := rs.Scan(&key, &name, &typ, &cmc, &qty); err != nil {
				rs.Close()
				serverErr(w, err)
				return
			}
			row := byKey[key]
			if row == nil {
				row = &compareRow{Key: key, Name: name, CMC: nullPtr(cmc), Qty: make([]int, len(ids))}
				if typ.Valid {
					t := typ.String
					row.Type = &t
				}
				byKey[key] = row
				rows = append(rows, row)
			}
			row.Qty[i] += qty
		}
		rs.Close()
	}
	writeJSON(w, http.StatusOK, map[string]any{"decks": decks, "cards": rows})
}
