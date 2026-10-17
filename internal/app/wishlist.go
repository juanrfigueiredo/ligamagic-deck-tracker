package app

import (
	"context"
	"database/sql"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Preço de 1 cópia de uma carta num dia, juntando todos os decks acompanhados em que ela aparece.
type unitPoint struct {
	Day string `json:"day"`
	Min *int64 `json:"min"`
	Avg *int64 `json:"avg"`
	Max *int64 `json:"max"`
}

type wishItem struct {
	Key      string      `json:"key"`
	Name     string      `json:"name"`
	Type     string      `json:"type"`
	Cmc      *int64      `json:"cmc"`
	Mana     string      `json:"mana"`
	Qty      int64       `json:"qty"`
	AddedDay string      `json:"added_day"`
	AddedAvg *int64      `json:"added_avg"` // preço médio de 1 cópia quando entrou na lista
	Day      string      `json:"day"`       // último dia com preço
	Avg      *int64      `json:"avg"`
	Min      *int64      `json:"min"`
	PrevAvg  *int64      `json:"prev_avg"` // dia anterior com preço
	LowAvg   *int64      `json:"low_avg"`  // menor preço médio desde que entrou
	Points   []unitPoint `json:"points"`
}

// unitHistory devolve, por carta, o preço de 1 cópia por dia (média entre os decks em que aparece).
func (a *App) unitHistory(ctx context.Context, keys []string) (map[string][]unitPoint, error) {
	out := map[string][]unitPoint{}
	if len(keys) == 0 {
		return out, nil
	}
	args := make([]any, len(keys))
	for i, k := range keys {
		args[i] = k
	}
	rows, err := a.db.QueryContext(ctx, `
		SELECT sc.card_key, s.day,
		       CAST(MIN(sc.price_min * 1.0 / sc.qty) AS INTEGER),
		       CAST(AVG(sc.price_avg * 1.0 / sc.qty) AS INTEGER),
		       CAST(MAX(sc.price_max * 1.0 / sc.qty) AS INTEGER)
		FROM snapshot_cards sc JOIN snapshots s ON s.id = sc.snapshot_id
		WHERE sc.card_key IN (`+strings.TrimSuffix(strings.Repeat("?, ", len(keys)), ", ")+`) AND sc.qty > 0
		GROUP BY sc.card_key, s.day ORDER BY s.day`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var p unitPoint
		var mn, av, mx sql.NullInt64
		if err := rows.Scan(&k, &p.Day, &mn, &av, &mx); err != nil {
			return nil, err
		}
		p.Min, p.Avg, p.Max = nullPtr(mn), nullPtr(av), nullPtr(mx)
		out[k] = append(out[k], p)
	}
	return out, rows.Err()
}

// GET /api/wishlist
func (a *App) handleWishlist(w http.ResponseWriter, r *http.Request, u user) {
	ctx := r.Context()
	rows, err := a.db.QueryContext(ctx, `
		SELECT w.card_key, c.name, COALESCE(ci.type, ''), ci.cmc, COALESCE(ci.mana, ''), w.qty, w.added_avg, w.created_at
		FROM wishlist w JOIN cards c ON c.card_key = w.card_key
		LEFT JOIN card_info ci ON ci.card_key = w.card_key
		WHERE w.user_id = ?`, u.ID)
	if err != nil {
		serverErr(w, err)
		return
	}
	items := []wishItem{}
	for rows.Next() {
		var it wishItem
		var cmc, added sql.NullInt64
		var created int64
		if err := rows.Scan(&it.Key, &it.Name, &it.Type, &cmc, &it.Mana, &it.Qty, &added, &created); err != nil {
			rows.Close()
			serverErr(w, err)
			return
		}
		it.Cmc, it.AddedAvg = nullPtr(cmc), nullPtr(added)
		it.AddedDay = time.Unix(created, 0).In(a.loc).Format("2006-01-02")
		items = append(items, it)
	}
	rows.Close()

	keys := make([]string, len(items))
	for i, it := range items {
		keys[i] = it.Key
	}
	hist, err := a.unitHistory(ctx, keys)
	if err != nil {
		serverErr(w, err)
		return
	}
	for i := range items {
		it := &items[i]
		var priced []unitPoint
		for _, p := range hist[it.Key] {
			if p.Avg != nil {
				priced = append(priced, p)
			}
		}
		it.Points = hist[it.Key]
		if it.Points == nil {
			it.Points = []unitPoint{}
		}
		if n := len(priced); n > 0 {
			last := priced[n-1]
			it.Day, it.Avg, it.Min = last.Day, last.Avg, last.Min
			if n > 1 {
				it.PrevAvg = priced[n-2].Avg
			}
		}
		for _, p := range priced {
			if p.Day >= it.AddedDay && (it.LowAvg == nil || *p.Avg < *it.LowAvg) {
				it.LowAvg = p.Avg
			}
		}
	}
	// Quem mais caiu desde que entrou vem primeiro.
	sort.SliceStable(items, func(i, j int) bool { return dropPct(items[i]) < dropPct(items[j]) })
	writeJSON(w, http.StatusOK, items)
}

func dropPct(it wishItem) float64 {
	if it.Avg == nil || it.AddedAvg == nil || *it.AddedAvg == 0 {
		return 0
	}
	return float64(*it.Avg-*it.AddedAvg) / float64(*it.AddedAvg)
}

// POST /api/wishlist {items: [{key, qty}]}: adiciona ou atualiza a quantidade. O preço de entrada fica o da primeira vez.
func (a *App) handleWishlistAdd(w http.ResponseWriter, r *http.Request, u user) {
	var in struct {
		Items []struct {
			Key string `json:"key"`
			Qty int64  `json:"qty"`
		} `json:"items"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if len(in.Items) == 0 || len(in.Items) > 500 {
		writeErr(w, http.StatusBadRequest, "Escolha as cartas.")
		return
	}
	ctx := r.Context()
	keys := make([]string, 0, len(in.Items))
	for _, it := range in.Items {
		keys = append(keys, strings.TrimSpace(it.Key))
	}
	hist, err := a.unitHistory(ctx, keys)
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
	added := 0
	for i, it := range in.Items {
		key := keys[i]
		if it.Qty < 1 || it.Qty > 99 {
			it.Qty = 1
		}
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM cards WHERE card_key = ?`, key).Scan(&exists); err != nil {
			serverErr(w, err)
			return
		}
		if exists == 0 {
			continue
		}
		var price *int64
		if pts := hist[key]; len(pts) > 0 {
			price = pts[len(pts)-1].Avg
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO wishlist (user_id, card_key, qty, added_avg, created_at) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (user_id, card_key) DO UPDATE SET qty = excluded.qty`,
			u.ID, key, it.Qty, price, now); err != nil {
			serverErr(w, err)
			return
		}
		added++
	}
	if err := tx.Commit(); err != nil {
		serverErr(w, err)
		return
	}
	if added == 0 {
		writeErr(w, http.StatusNotFound, "Nenhuma dessas cartas tem preço salvo ainda.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "added": added})
}

// DELETE /api/wishlist/{key}
func (a *App) handleWishlistRemove(w http.ResponseWriter, r *http.Request, u user) {
	key := r.PathValue("key")
	res, err := a.db.ExecContext(r.Context(), `DELETE FROM wishlist WHERE user_id = ? AND card_key = ?`, u.ID, key)
	if err != nil {
		serverErr(w, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeErr(w, http.StatusNotFound, "Carta não está na lista.")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
