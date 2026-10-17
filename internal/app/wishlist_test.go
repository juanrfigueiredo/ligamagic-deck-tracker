package app

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"
)

func TestWishlist(t *testing.T) {
	srv, m, a := setupApp(t)
	day := time.Date(2026, 10, 1, 12, 0, 0, 0, a.loc)
	a.now = func() time.Time { return day }
	tok := login(t, srv, m, "juan@example.com")
	do(t, srv, "POST", "/api/decks", tok, map[string]string{"deck": "1"})
	snap := func(goblin int64) snapIn {
		s := snapIn{Name: "A", Cards: []snapCard{
			{Key: "g", Name: "Goblin", Board: "main", Qty: 2, Avg: i64(goblin * 2), Type: "Criaturas", Cmc: i64(1), Mana: strp("R")},
			{Key: "b", Name: "Bolt", Board: "main", Qty: 1, Avg: i64(500)},
		}}
		s.Totals.Avg = i64(goblin*2 + 500)
		return s
	}
	if st, b := do(t, srv, "POST", "/api/decks/1/snapshot", tok, snap(1000)); st != 200 {
		t.Fatalf("snapshot: %d %v", st, b)
	}
	if st, b := do(t, srv, "POST", "/api/wishlist", tok, map[string]any{"items": []map[string]any{{"key": "g", "qty": 2}, {"key": "b"}, {"key": "nao-existe"}}}); st != 200 || b["added"] != 2.0 {
		t.Fatalf("add: %d %v", st, b)
	}
	day = day.AddDate(0, 0, 1)
	do(t, srv, "POST", "/api/decks/1/snapshot", tok, snap(700))
	// Adicionar de novo não troca o preço de entrada.
	do(t, srv, "POST", "/api/wishlist", tok, map[string]any{"items": []map[string]any{{"key": "g", "qty": 3}}})

	items := wishlist(t, srv, tok)
	if len(items) != 2 || items[0]["name"] != "Goblin" {
		t.Fatalf("ordem (quem caiu primeiro): %v", items)
	}
	g := items[0]
	if g["added_avg"] != 1000.0 || g["avg"] != 700.0 || g["prev_avg"] != 1000.0 || g["low_avg"] != 700.0 || g["qty"] != 3.0 || g["mana"] != "R" {
		t.Fatalf("Goblin: %v", g)
	}
	if st, _ := do(t, srv, "DELETE", "/api/wishlist/b", tok, nil); st != 200 {
		t.Fatalf("remove: %d", st)
	}
	if st, _ := do(t, srv, "DELETE", "/api/wishlist/b", tok, nil); st != 404 {
		t.Fatalf("remove 2x: %d", st)
	}
	if items := wishlist(t, srv, tok); len(items) != 1 {
		t.Fatalf("depois de remover: %v", items)
	}
}

func wishlist(t *testing.T, srv *httptest.Server, tok string) []map[string]any {
	t.Helper()
	st, b := do(t, srv, "GET", "/api/wishlist", tok, nil)
	raw, ok := b["list"].(json.RawMessage)
	if st != 200 || !ok {
		t.Fatalf("lista: %d %v", st, b)
	}
	var out []map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
