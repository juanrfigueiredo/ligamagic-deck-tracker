package app

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func snapWith(avg int64) snapIn {
	s := snapIn{Name: "Burn", Cards: []snapCard{
		{Key: "1", Name: "Lightning Bolt", Board: "main", Qty: 4, Min: i64(avg / 2), Avg: i64(avg), Max: i64(avg * 2)},
		{Key: "2", Name: "Mountain", Board: "main", Qty: 20, Min: i64(100), Avg: i64(100), Max: i64(100)},
	}}
	s.Totals.Min, s.Totals.Avg, s.Totals.Max = i64(avg/2+100), i64(avg+100), i64(avg*2+100)
	return s
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

func TestAlertsShareCSV(t *testing.T) {
	srv, m, a := setupApp(t)
	day1 := time.Date(2026, 10, 10, 15, 0, 0, 0, time.UTC)
	a.now = func() time.Time { return day1 }
	tok := login(t, srv, m, "a@b.com")
	do(t, srv, "POST", "/api/decks", tok, map[string]string{"deck": "42"})

	if st, b := do(t, srv, "PUT", "/api/decks/42/alert", tok, map[string]any{"pct": 0}); st != 400 {
		t.Fatalf("alerta 0%% aceito: %d %v", st, b)
	}
	if st, b := do(t, srv, "PUT", "/api/decks/42/alert", tok, map[string]any{"pct": 10}); st != 200 {
		t.Fatalf("alerta: %d %v", st, b)
	}
	if st, b := do(t, srv, "POST", "/api/decks/42/snapshot", tok, snapWith(10000)); st != 200 {
		t.Fatalf("snapshot dia 1: %d %v", st, b)
	}
	_, b := do(t, srv, "GET", "/api/decks", tok, nil)
	if !strings.Contains(string(b["list"].(json.RawMessage)), `"snapshot_today":true`) {
		t.Fatalf("snapshot_today deveria ser true: %s", b["list"])
	}

	// Dia 2: +20% no deck dispara o alerta, uma vez só.
	a.now = func() time.Time { return day1.Add(24 * time.Hour) }
	_, b = do(t, srv, "GET", "/api/decks", tok, nil)
	if strings.Contains(string(b["list"].(json.RawMessage)), `"snapshot_today":true`) {
		t.Fatal("snapshot_today deveria ser false no dia seguinte")
	}
	sent := m.sent
	do(t, srv, "POST", "/api/decks/42/snapshot", tok, snapWith(12000))
	if m.sent != sent+1 || m.lastTo != "a@b.com" || !strings.Contains(m.lastSubject, "subiu +19,8%") || !strings.Contains(m.last, "Lightning Bolt") {
		t.Fatalf("alerta não enviado como esperado: %d %q %q", m.sent-sent, m.lastSubject, m.last)
	}
	do(t, srv, "POST", "/api/decks/42/snapshot", tok, snapWith(15000))
	if m.sent != sent+1 {
		t.Fatal("alerta repetido no mesmo dia")
	}
	_, b = do(t, srv, "GET", "/api/decks/42/history", tok, nil)
	if b["alert_pct"].(float64) != 10 || b["share_url"] != nil {
		t.Fatalf("history do dono: %v", b)
	}

	// Link público: abre sem login, não mostra dados da conta, e some ao desativar.
	_, b = do(t, srv, "POST", "/api/decks/42/share", tok, nil)
	token := b["token"].(string)
	_, b2 := do(t, srv, "POST", "/api/decks/42/share", tok, nil)
	if b2["token"] != token || !strings.HasSuffix(b["url"].(string), "/#/p/"+token) {
		t.Fatalf("link deveria ser reaproveitado: %v %v", b, b2)
	}
	st, b := do(t, srv, "GET", "/api/public/"+token+"/history", "", nil)
	if st != 200 || len(b["points"].([]any)) != 2 {
		t.Fatalf("público: %d %v", st, b)
	}
	if _, ok := b["alert_pct"]; ok {
		t.Fatal("link público expõe o alerta do dono")
	}
	if st, b := do(t, srv, "GET", "/api/public/"+token+"/cards", "", nil); st != 200 || len(b["cards"].([]any)) != 2 {
		t.Fatalf("cartas públicas: %d %v", st, b)
	}

	// CSV
	st, body := get(t, srv.URL+"/api/public/"+token+"/export.csv")
	lines := strings.Split(strings.TrimSpace(strings.TrimPrefix(body, "\ufeff")), "\n")
	if st != 200 || lines[0] != "data;tipo;board;carta;qtd;minimo;medio;maximo" || len(lines) != 7 {
		t.Fatalf("csv: %d\n%s", st, body)
	}
	if lines[1] != "2026-10-10;TOTAL;;;;51,00;101,00;201,00" || !strings.HasPrefix(lines[2], "2026-10-10;carta;main;Lightning Bolt;4;50,00;100,00;200,00") {
		t.Fatalf("csv linhas: %q %q", lines[1], lines[2])
	}

	do(t, srv, "DELETE", "/api/decks/42/share", tok, nil)
	if st, _ := do(t, srv, "GET", "/api/public/"+token+"/history", "", nil); st != 404 {
		t.Fatalf("link desativado ainda abre: %d", st)
	}
	if st, _ := do(t, srv, "GET", "/api/decks/42/export.csv", "", nil); st != 401 {
		t.Fatalf("csv privado sem login: %d", st)
	}
}

func TestAccountExportDeleteAdmin(t *testing.T) {
	srv, m, a := setupApp(t)
	a.SetAdmins(" Juan@Admin.com , lixo")
	user := login(t, srv, m, "user@b.com")
	admin := login(t, srv, m, "juan@admin.com")
	do(t, srv, "POST", "/api/decks", user, map[string]string{"deck": "7"})
	do(t, srv, "POST", "/api/decks/7/snapshot", user, snapWith(5000))
	do(t, srv, "PUT", "/api/decks/7/alert", user, map[string]any{"pct": 5})

	if _, b := do(t, srv, "GET", "/api/me", user, nil); b["admin"] != false {
		t.Fatalf("usuário comum como admin: %v", b)
	}
	if _, b := do(t, srv, "GET", "/api/me", admin, nil); b["admin"] != true {
		t.Fatalf("admin não reconhecido: %v", b)
	}
	if st, _ := do(t, srv, "GET", "/api/admin/stats", user, nil); st != 403 {
		t.Fatalf("stats para não admin: %d", st)
	}
	st, b := do(t, srv, "GET", "/api/admin/stats", admin, nil)
	tot := b["totals"].(map[string]any)
	if st != 200 || tot["users"].(float64) != 2 || tot["snapshots"].(float64) != 1 || len(b["snapshots_per_day"].([]any)) != 31 || len(b["top_decks"].([]any)) != 1 {
		t.Fatalf("stats: %d %v", st, b)
	}

	st, b = do(t, srv, "GET", "/api/me/export", user, nil)
	decks := b["decks"].([]any)
	if st != 200 || b["email"] != "user@b.com" || len(decks) != 1 || decks[0].(map[string]any)["alert_pct"].(float64) != 5 {
		t.Fatalf("export: %d %v", st, b)
	}

	if st, _ := do(t, srv, "DELETE", "/api/me", user, map[string]string{"confirm": "outro@b.com"}); st != 400 {
		t.Fatalf("excluiu sem confirmar: %d", st)
	}
	if st, _ := do(t, srv, "DELETE", "/api/me", user, map[string]string{"confirm": "USER@b.com"}); st != 200 {
		t.Fatalf("excluir: %d", st)
	}
	if st, _ := do(t, srv, "GET", "/api/me", user, nil); st != 401 {
		t.Fatalf("sessão sobreviveu à exclusão: %d", st)
	}
	_, b = do(t, srv, "GET", "/api/admin/stats", admin, nil)
	tot = b["totals"].(map[string]any)
	if tot["users"].(float64) != 1 || tot["alerts"].(float64) != 0 || tot["follows"].(float64) != 0 || tot["snapshots"].(float64) != 1 {
		t.Fatalf("dados que sobraram: %v", tot)
	}
}

func TestExtensionInfo(t *testing.T) {
	srv, _ := setup(t)
	if st, b := do(t, srv, "GET", "/api/extension", "", nil); st != 200 || b["version"] != extVersion || extVersion == "" {
		t.Fatalf("versão: %d %v", st, b)
	}
}

func TestFormat(t *testing.T) {
	for in, want := range map[int64]string{5: "R$ 0,05", 177358: "R$ 1.773,58", 123456789: "R$ 1.234.567,89"} {
		if got := fmtBRL(in); got != want {
			t.Errorf("fmtBRL(%d) = %s", in, got)
		}
	}
	if fmtPct(-3.25) != "-3,2%" && fmtPct(-3.25) != "-3,3%" {
		t.Errorf("fmtPct: %s", fmtPct(-3.25))
	}
}
