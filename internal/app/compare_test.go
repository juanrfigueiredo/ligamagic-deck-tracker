package app

import "testing"

func s(v string) *string { return &v }

func TestCompare(t *testing.T) {
	srv, m := setup(t)
	tok := login(t, srv, m, "juan@example.com")
	for _, id := range []string{"1", "2", "3"} {
		if st, b := do(t, srv, "POST", "/api/decks", tok, map[string]string{"deck": id}); st != 201 {
			t.Fatalf("add %s: %d %v", id, st, b)
		}
	}
	post := func(id string, cards []snapCard) {
		t.Helper()
		if st, b := do(t, srv, "POST", "/api/decks/"+id+"/snapshot", tok, snapIn{Name: "D" + id, Cards: cards}); st != 200 {
			t.Fatalf("snapshot %s: %d %v", id, st, b)
		}
	}
	post("1", []snapCard{
		{Key: "a", Name: "Raio", Board: "main", Qty: 1, Type: s("Mágicas"), CMC: i64(1)},
		{Key: "b", Name: "Fúria", Board: "main", Qty: 1, Type: s("Criaturas"), CMC: i64(5)},
		{Key: "m", Name: "Montanha", Board: "main", Qty: 27, Type: s("Terrenos"), CMC: i64(0)},
		{Key: "x", Name: "Choque", Board: "maybe", Qty: 1},
	})
	// Versão antiga da extensão (sem tipo/custo) não apaga o que já foi salvo.
	post("2", []snapCard{
		{Key: "a", Name: "Raio", Board: "main", Qty: 1},
		{Key: "m", Name: "Montanha", Board: "main", Qty: 25},
		{Key: "c", Name: "Sear", Board: "main", Qty: 1, Type: s("Mágicas"), CMC: i64(2)},
	})

	if st, _ := do(t, srv, "GET", "/api/compare?decks=1", tok, nil); st != 400 {
		t.Fatalf("1 deck aceito: %d", st)
	}
	if st, _ := do(t, srv, "GET", "/api/compare?decks=1,2,3,4,5", tok, nil); st != 400 {
		t.Fatalf("5 decks aceitos: %d", st)
	}
	if st, _ := do(t, srv, "GET", "/api/compare?decks=1,999", tok, nil); st != 404 {
		t.Fatalf("deck não acompanhado aceito: %d", st)
	}
	st, b := do(t, srv, "GET", "/api/compare?decks=1,2,3", tok, nil)
	if st != 200 {
		t.Fatalf("compare: %d %v", st, b)
	}
	rows := map[string]map[string]any{}
	for _, r := range b["cards"].([]any) {
		row := r.(map[string]any)
		rows[row["key"].(string)] = row
	}
	if len(rows) != 4 || rows["x"] != nil {
		t.Fatalf("cartas erradas (maybeboard deve ficar de fora): %v", rows)
	}
	q := func(k string) []any { return rows[k]["qty"].([]any) }
	if a := q("a"); a[0].(float64) != 1 || a[1].(float64) != 1 || a[2].(float64) != 0 {
		t.Fatalf("Raio: %v", a)
	}
	if mt := q("m"); mt[0].(float64) != 27 || mt[1].(float64) != 25 {
		t.Fatalf("Montanha: %v", mt)
	}
	if rows["a"]["type"] != "Mágicas" || rows["a"]["cmc"].(float64) != 1 {
		t.Fatalf("tipo/custo perdidos: %v", rows["a"])
	}
}
