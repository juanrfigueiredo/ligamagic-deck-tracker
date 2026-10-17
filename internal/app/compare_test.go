package app

import "testing"

func strp(s string) *string { return &s }

func TestCompare(t *testing.T) {
	srv, m := setup(t)
	tok := login(t, srv, m, "juan@example.com")
	for _, id := range []string{"1", "2"} {
		do(t, srv, "POST", "/api/decks", tok, map[string]string{"deck": id})
	}
	if st, _ := do(t, srv, "GET", "/api/compare?a=1&b=2", tok, nil); st != 422 {
		t.Fatalf("sem snapshot deveria dar 422: %d", st)
	}
	card := func(key, name, typ string, qty, cmc, avg int64, mana string) snapCard {
		return snapCard{Key: key, Name: name, Board: "main", Qty: int(qty), Avg: i64(avg), Type: typ, Cmc: i64(cmc), Mana: strp(mana)}
	}
	a := snapIn{Name: "A", Cards: []snapCard{
		{Key: "z", Name: "Zurgo", Board: "main", Qty: 1, Avg: i64(100), Type: "Comandante", Cmc: i64(1), Mana: strp("R")},
		card("b", "Bolt", "Mágicas", 1, 1, 500, "R"),
		card("m", "Montanha", "Terrenos", 20, 0, 0, ""),
		{Key: "x", Name: "Só no maybe", Board: "maybe", Qty: 1, Type: "Maybeboard"},
	}}
	a.Totals.Avg = i64(600)
	b := snapIn{Name: "B", Cards: []snapCard{
		card("z", "Zurgo", "Criaturas", 1, 1, 100, "R"),
		card("g", "Goblin", "Criaturas", 1, 1, 300, "R"),
		card("m", "Montanha", "Terrenos", 25, 0, 0, ""),
	}}
	b.Totals.Avg = i64(400)
	for id, s := range map[string]snapIn{"1": a, "2": b} {
		if st, body := do(t, srv, "POST", "/api/decks/"+id+"/snapshot", tok, s); st != 200 {
			t.Fatalf("snapshot %s: %d %v", id, st, body)
		}
	}
	st, body := do(t, srv, "GET", "/api/compare?a=1&b=2", tok, nil)
	if st != 200 {
		t.Fatalf("compare: %d %v", st, body)
	}
	got := map[string]map[string]any{}
	for _, c := range body["cards"].([]any) {
		c := c.(map[string]any)
		got[c["name"].(string)] = c
	}
	if len(got) != 4 {
		t.Fatalf("cartas: %v", got)
	}
	if g := got["Bolt"]; g["qty_a"] != 1.0 || g["qty_b"] != 0.0 || g["unit_avg"] != 500.0 {
		t.Fatalf("Bolt: %v", g)
	}
	if g := got["Montanha"]; g["qty_a"] != 20.0 || g["qty_b"] != 25.0 || g["cmc"] != 0.0 {
		t.Fatalf("Montanha: %v", g)
	}
	// "Comandante" não é tipo: fica o tipo vindo do outro deck.
	if g := got["Zurgo"]; g["type"] != "Criaturas" || g["mana"] != "R" {
		t.Fatalf("Zurgo: %v", g)
	}
	if _, ok := got["Só no maybe"]; ok {
		t.Fatal("maybeboard entrou na comparação")
	}
	if st, _ := do(t, srv, "GET", "/api/compare?a=1&b=1", tok, nil); st != 400 {
		t.Fatalf("mesmo deck: %d", st)
	}
	tok2 := login(t, srv, m, "outro@example.com")
	if st, _ := do(t, srv, "GET", "/api/compare?a=1&b=2", tok2, nil); st != 404 {
		t.Fatalf("deck de outro usuário: %d", st)
	}
}
