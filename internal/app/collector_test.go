package app

import (
	"net/http"
	"strings"
	"testing"
)

func TestCollectorKey(t *testing.T) {
	srv, m := setup(t)
	tok := login(t, srv, m, "juan@example.com")

	if _, b := do(t, srv, "GET", "/api/me/key", tok, nil); b["exists"] != false {
		t.Fatalf("chave não deveria existir: %v", b)
	}
	_, b := do(t, srv, "POST", "/api/me/key", tok, nil)
	key, _ := b["key"].(string)
	if !strings.HasPrefix(key, keyPrefix) {
		t.Fatalf("chave: %v", b)
	}

	// Rotas de coleta aceitam a chave.
	if st, b := do(t, srv, "POST", "/api/decks", key, map[string]string{"deck": "123"}); st != 201 {
		t.Fatalf("add com chave: %d %v", st, b)
	}
	if st, b := do(t, srv, "GET", "/api/decks/123/status", key, nil); st != 200 || b["tracked"] != true {
		t.Fatalf("status com chave: %d %v", st, b)
	}
	if st, b := do(t, srv, "POST", "/api/decks/123/snapshot", key, snapWith(1000)); st != 200 {
		t.Fatalf("snapshot com chave: %d %v", st, b)
	}
	if st, _ := do(t, srv, "GET", "/api/decks", key, nil); st != 200 {
		t.Fatalf("lista com chave: %d", st)
	}
	// O resto não.
	for _, p := range []string{"/api/me", "/api/me/export", "/api/decks/123/history"} {
		if st, _ := do(t, srv, "GET", p, key, nil); st != 401 {
			t.Fatalf("%s aceitou a chave: %d", p, st)
		}
	}
	if _, b := do(t, srv, "GET", "/api/me/key", tok, nil); b["exists"] != true || b["last_used_at"] == nil {
		t.Fatalf("info: %v", b)
	}

	// Gerar outra invalida a anterior.
	_, b = do(t, srv, "POST", "/api/me/key", tok, nil)
	if st, _ := do(t, srv, "GET", "/api/decks", key, nil); st != 401 {
		t.Fatalf("chave antiga ainda vale: %d", st)
	}
	key2 := b["key"].(string)
	do(t, srv, "DELETE", "/api/me/key", tok, nil)
	if st, _ := do(t, srv, "GET", "/api/decks", key2, nil); st != 401 {
		t.Fatalf("chave revogada ainda vale: %d", st)
	}
}

func TestCORS(t *testing.T) {
	srv, _ := setup(t)
	pre := func(origin, path string) *http.Response {
		req, _ := http.NewRequest("OPTIONS", srv.URL+path, nil)
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", "POST")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res
	}
	res := pre("https://www.ligamagic.com.br", "/api/decks/1/snapshot")
	if res.StatusCode != 204 || res.Header.Get("Access-Control-Allow-Origin") != "https://www.ligamagic.com.br" {
		t.Fatalf("preflight LigaMagic: %d %v", res.StatusCode, res.Header)
	}
	if res := pre("https://evil.example", "/api/decks/1/snapshot"); res.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("CORS liberado para outra origem")
	}
	if res := pre("https://www.ligamagic.com.br", "/api/me"); res.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("CORS liberado fora das rotas de coleta")
	}
}
