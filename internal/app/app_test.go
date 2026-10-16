package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"testing"
)

type fakeMail struct {
	last, lastTo, lastSubject string
	sent                      int
}

func (f *fakeMail) Send(_ context.Context, to, subject, text, html string) error {
	f.last, f.lastTo, f.lastSubject = text, to, subject
	f.sent++
	return nil
}

func setup(t *testing.T) (*httptest.Server, *fakeMail) {
	t.Helper()
	srv, m, _ := setupApp(t)
	return srv, m
}

func setupApp(t *testing.T) (*httptest.Server, *fakeMail, *App) {
	t.Helper()
	db, err := OpenDB("file:"+filepath.Join(t.TempDir(), "test.db"), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	m := &fakeMail{}
	a := New(db, m, "test", "http://x")
	srv := httptest.NewServer(a.Routes())
	t.Cleanup(func() { srv.Close(); db.Close() })
	return srv, m, a
}

func do(t *testing.T, srv *httptest.Server, method, path, token string, body any) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, srv.URL+path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	var raw json.RawMessage
	json.NewDecoder(res.Body).Decode(&raw)
	if json.Unmarshal(raw, &out) != nil {
		out = map[string]any{"list": raw}
	}
	return res.StatusCode, out
}

var codeRe = regexp.MustCompile(`\d{6}`)

func login(t *testing.T, srv *httptest.Server, m *fakeMail, email string) string {
	t.Helper()
	if st, b := do(t, srv, "POST", "/api/auth/request", "", map[string]string{"email": email}); st != 200 {
		t.Fatalf("request: %d %v", st, b)
	}
	code := codeRe.FindString(m.last)
	st, b := do(t, srv, "POST", "/api/auth/verify", "", map[string]string{"email": email, "code": code})
	if st != 200 {
		t.Fatalf("verify: %d %v", st, b)
	}
	// Uso único: o mesmo código não entra de novo.
	if st, _ := do(t, srv, "POST", "/api/auth/verify", "", map[string]string{"email": email, "code": code}); st != 400 {
		t.Fatalf("código reutilizado aceito: %d", st)
	}
	return b["token"].(string)
}

func i64(v int64) *int64 { return &v }

func TestFlow(t *testing.T) {
	srv, m := setup(t)
	tok := login(t, srv, m, "Juan@Example.com")

	if st, _ := do(t, srv, "POST", "/api/auth/request", "", map[string]string{"email": "juan@example.com"}); st != 429 {
		t.Fatalf("cooldown não aplicado: %d", st)
	}

	if st, b := do(t, srv, "POST", "/api/decks", tok, map[string]string{"deck": "https://www.ligamagic.com.br/?view=dks/deck&id=10256897"}); st != 201 {
		t.Fatalf("add: %d %v", st, b)
	}
	snap := snapIn{Name: "Zurgo", Cards: []snapCard{
		{Key: "1", Name: "Zurgo", Board: "main", Qty: 1},
		{Key: "2", Name: "Canalizadora", Board: "main", Qty: 1, Min: i64(970), Avg: i64(2186), Max: i64(4949)},
		{Key: "2", Name: "Canalizadora", Board: "main", Qty: 1, Min: i64(100), Avg: i64(200), Max: i64(300)},
		{Key: "3", Name: "Montanha", Board: "maybe", Qty: 27},
	}}
	snap.Totals.Min, snap.Totals.Avg, snap.Totals.Max = i64(1070), i64(2386), i64(5249)
	for i := 0; i < 2; i++ { // segundo envio no mesmo dia atualiza, não duplica
		if st, b := do(t, srv, "POST", "/api/decks/10256897/snapshot", tok, snap); st != 200 || b["cards"].(float64) != 3 {
			t.Fatalf("snapshot: %d %v", st, b)
		}
	}
	st, b := do(t, srv, "GET", "/api/decks/10256897/history", tok, nil)
	if st != 200 || len(b["points"].([]any)) != 1 || b["name"] != "Zurgo" {
		t.Fatalf("history: %d %v", st, b)
	}
	st, b = do(t, srv, "GET", "/api/decks/10256897/cards", tok, nil)
	if st != 200 || len(b["cards"].([]any)) != 3 {
		t.Fatalf("cards: %d %v", st, b)
	}
	first := b["cards"].([]any)[0].(map[string]any)
	if first["name"] != "Canalizadora" || first["avg"].(float64) != 2386 || first["qty"].(float64) != 2 {
		t.Fatalf("merge errado: %v", first)
	}
	if st, b := do(t, srv, "GET", "/api/decks/10256897/card-history?key=2", tok, nil); st != 200 || len(b["points"].([]any)) != 1 {
		t.Fatalf("card history: %d %v", st, b)
	}

	// Transferência (mover) para outro e-mail
	if st, b := do(t, srv, "POST", "/api/transfer", tok, map[string]string{"email": "outro@example.com", "mode": "move"}); st != 200 {
		t.Fatalf("transfer: %d %v", st, b)
	}
	if st, _ := do(t, srv, "GET", "/api/decks/10256897/history", tok, nil); st != 404 {
		t.Fatalf("deck deveria ter saído da conta: %d", st)
	}
	tok2 := login(t, srv, m, "outro@example.com")
	if st, b := do(t, srv, "GET", "/api/decks/10256897/history", tok2, nil); st != 200 || len(b["points"].([]any)) != 1 {
		t.Fatalf("destino sem histórico: %d %v", st, b)
	}

	// Logout invalida o token
	do(t, srv, "POST", "/api/auth/logout", tok2, nil)
	if st, _ := do(t, srv, "GET", "/api/me", tok2, nil); st != 401 {
		t.Fatalf("token ainda válido após logout: %d", st)
	}
}

func TestWrongCodeAttempts(t *testing.T) {
	srv, m := setup(t)
	do(t, srv, "POST", "/api/auth/request", "", map[string]string{"email": "a@b.com"})
	code := codeRe.FindString(m.last)
	wrong := "000000"
	if code == wrong {
		wrong = "111111"
	}
	for i := 0; i < maxAttempts; i++ {
		do(t, srv, "POST", "/api/auth/verify", "", map[string]string{"email": "a@b.com", "code": wrong})
	}
	if st, _ := do(t, srv, "POST", "/api/auth/verify", "", map[string]string{"email": "a@b.com", "code": code}); st != 400 {
		t.Fatalf("código certo aceito depois de %d erros: %d", maxAttempts, st)
	}
}

func TestParseDeckRef(t *testing.T) {
	for in, want := range map[string]int64{
		"10256897": 10256897,
		"https://www.ligamagic.com.br/?view=dks/deck&id=10256897": 10256897,
		"https://www.ligamagic.com.br/?id=55&view=dks/deck":       55,
	} {
		if got, ok := parseDeckRef(in); !ok || got != want {
			t.Errorf("%s: %d %v", in, got, ok)
		}
	}
	if _, ok := parseDeckRef("abc"); ok {
		t.Error("aceitou abc")
	}
}

func TestExtensionZip(t *testing.T) {
	data, err := buildExtensionZip()
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	for _, n := range []string{"manifest.json", "background.js", "content.js", "popup.html", "popup.js", "site.js", "icons/128.png"} {
		if !names["mtg-tracker/"+n] {
			t.Errorf("zip sem %s: %v", n, names)
		}
	}
}
