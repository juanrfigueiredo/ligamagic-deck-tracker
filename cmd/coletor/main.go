// Coletor do MTG Tracker: roda no seu computador (IP brasileiro, Chrome de verdade),
// abre os decks acompanhados que ainda não têm o preço do dia e envia para o site.
//
// Primeira vez: dê dois cliques no executável e cole a chave de coleta (gerada no site, em Ajuda).
// Ele salva a configuração, oferece agendar para rodar de hora em hora e já faz a primeira coleta.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"

	"github.com/juanrfigueiredo/ligamagic-deck-tracker/extension"
)

const (
	defaultServer = "https://mtg.jrfsoftware.com.br"
	taskName      = "MTG Tracker Coletor"
	pageTimeout   = 45 * time.Second
)

type config struct {
	Server string `json:"servidor"`
	Key    string `json:"chave"`
}

type deck struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	URL           string `json:"url"`
	SnapshotToday bool   `json:"snapshot_today"`
}

func main() {
	var (
		key      = flag.String("chave", "", "chave de coleta (gerada no site, em Ajuda); fica salva")
		server   = flag.String("servidor", "", "endereço do site (padrão "+defaultServer+"); fica salvo")
		visible  = flag.Bool("visivel", false, "abre o Chrome visível (útil se o Cloudflare barrar o modo oculto)")
		schedule = flag.Bool("agendar", false, "agenda o coletor para rodar de hora em hora e sai")
		unsched  = flag.Bool("desagendar", false, "remove o agendamento e sai")
		all      = flag.Bool("todos", false, "coleta todos os decks, mesmo os que já têm o preço de hoje")
		chrome   = flag.String("chrome", "", "caminho do Chrome/Edge, se não for encontrado sozinho")
		quiet    = flag.Bool("agendado", false, "modo usado pelo agendamento: sem perguntas, log em arquivo")
	)
	flag.Parse()

	dir, err := configDir()
	if err != nil {
		fatal(err)
	}
	if *quiet {
		f, err := os.OpenFile(filepath.Join(dir, "coletor.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err == nil {
			trimLog(f)
			log.SetOutput(f)
		}
	}

	if *unsched {
		if err := removeSchedule(); err != nil {
			fatal(err)
		}
		fmt.Println("Agendamento removido.")
		return
	}

	cfg := loadConfig(dir)
	changed := false
	if *key != "" {
		cfg.Key, changed = strings.TrimSpace(*key), true
	}
	if *server != "" {
		cfg.Server, changed = strings.TrimRight(strings.TrimSpace(*server), "/"), true
	}
	if cfg.Server == "" {
		cfg.Server = defaultServer
	}
	interactive := !*quiet && isTerminal()
	firstRun := cfg.Key == ""
	if firstRun {
		if !interactive {
			fatal(errors.New("sem chave de coleta: rode o coletor uma vez com -chave SUA_CHAVE"))
		}
		fmt.Println("MTG Tracker: coletor de preços")
		fmt.Println("Gere sua chave no site (" + cfg.Server + "/#/ajuda) e cole aqui.")
		cfg.Key = ask("Chave de coleta: ")
		changed = true
	}
	if !strings.HasPrefix(cfg.Key, "lmk_") {
		fatal(errors.New("chave inválida: ela começa com lmk_"))
	}
	if changed {
		if err := saveConfig(dir, cfg); err != nil {
			fatal(err)
		}
		log.Printf("configuração salva em %s", filepath.Join(dir, "config.json"))
	}

	if *schedule || (firstRun && interactive && yes(ask("Rodar sozinho de hora em hora? (S/n) "))) {
		if err := addSchedule(); err != nil {
			log.Printf("não consegui agendar: %v", err)
		} else if *schedule {
			return
		}
	}

	n, err := collect(cfg, dir, *chrome, *visible, *all)
	if err != nil {
		log.Printf("erro: %v", err)
	}
	if interactive {
		if err == nil && n == 0 {
			fmt.Println("Nada para coletar: todos os decks já têm o preço de hoje.")
		}
		ask("\nPressione Enter para sair.")
	}
	if err != nil {
		os.Exit(1)
	}
}

// collect devolve quantos decks tentou.
func collect(cfg config, dir, chromePath string, visible, all bool) (int, error) {
	var decks []deck
	if err := apiCall(cfg, "GET", "/api/decks", nil, &decks); err != nil {
		return 0, err
	}
	var pending []deck
	for _, d := range decks {
		if all || !d.SnapshotToday {
			pending = append(pending, d)
		}
	}
	if len(pending) == 0 {
		log.Printf("%d decks, todos com o preço de hoje", len(decks))
		return 0, nil
	}
	log.Printf("%d de %d decks sem o preço de hoje", len(pending), len(decks))

	parseJS, err := extension.Files.ReadFile("parse.js")
	if err != nil {
		return 0, err
	}
	if chromePath == "" {
		chromePath = findBrowser()
	}

	// Primeiro com o Chrome oculto. O que falhar (ex.: desafio do Cloudflare) tenta de novo com a janela fora da tela.
	failed := runBrowser(cfg, dir, chromePath, string(parseJS), pending, !visible)
	if len(failed) > 0 && !visible {
		log.Printf("%d falharam no modo oculto; tentando com o Chrome visível", len(failed))
		failed = runBrowser(cfg, dir, chromePath, string(parseJS), failed, false)
	}
	if len(failed) > 0 {
		names := make([]string, len(failed))
		for i, d := range failed {
			names[i] = deckLabel(d)
		}
		return len(pending), fmt.Errorf("não consegui coletar: %s", strings.Join(names, ", "))
	}
	return len(pending), nil
}

func runBrowser(cfg config, dir, chromePath, parseJS string, decks []deck, headless bool) (failed []deck) {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.UserDataDir(filepath.Join(dir, "chrome")), // perfil próprio: guarda o cookie do Cloudflare
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("lang", "pt-BR"),
		chromedp.WindowSize(1280, 900),
	)
	if headless {
		opts = append(opts, chromedp.Flag("headless", "new"))
	} else {
		opts = append(opts, chromedp.Flag("headless", false), chromedp.Flag("window-position", "-2400,-2400"))
	}
	if chromePath != "" {
		opts = append(opts, chromedp.ExecPath(chromePath))
	}
	actx, cancelA := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancelA()
	bctx, cancelB := chromedp.NewContext(actx)
	defer cancelB()

	// O modo oculto se anuncia como "HeadlessChrome" no user agent; troca pelo normal.
	if headless {
		var ua string
		if err := chromedp.Run(bctx, chromedp.Evaluate(`navigator.userAgent`, &ua)); err != nil {
			log.Printf("não consegui abrir o navegador: %v (use -chrome com o caminho do Chrome ou do Edge)", err)
			return decks
		}
		ua = strings.Replace(ua, "HeadlessChrome", "Chrome", 1)
		chromedp.Run(bctx, chromedp.ActionFunc(func(ctx context.Context) error {
			return emulation.SetUserAgentOverride(ua).WithAcceptLanguage("pt-BR,pt;q=0.9").Do(ctx)
		}))
	}

	for _, d := range decks {
		snap, err := readDeck(bctx, parseJS, d)
		if err != nil {
			log.Printf("%s: %v", deckLabel(d), err)
			failed = append(failed, d)
			continue
		}
		var res struct {
			Cards int `json:"cards"`
		}
		if err := apiCall(cfg, "POST", fmt.Sprintf("/api/decks/%d/snapshot", d.ID), snap, &res); err != nil {
			log.Printf("%s: %v", deckLabel(d), err)
			failed = append(failed, d)
			continue
		}
		log.Printf("%s: salvo, %s (médio), %d cartas", deckLabel(d), brl(snap.Totals.Avg), res.Cards)
	}
	return failed
}

type snapshot struct {
	Name   string `json:"name"`
	Totals struct {
		Min *int64 `json:"min"`
		Avg *int64 `json:"avg"`
		Max *int64 `json:"max"`
	} `json:"totals"`
	Cards []json.RawMessage `json:"cards"`
}

// readDeck abre o deck e espera os preços aparecerem (a página pode passar antes pelo desafio do Cloudflare).
func readDeck(bctx context.Context, parseJS string, d deck) (*snapshot, error) {
	ctx, cancel := context.WithTimeout(bctx, pageTimeout)
	defer cancel()
	if err := chromedp.Run(ctx, chromedp.Navigate(d.URL)); err != nil {
		return nil, fmt.Errorf("não abriu a página: %w", err)
	}
	// O parser é injetado a cada tentativa: o desafio do Cloudflare recarrega a página.
	expr := parseJS + "\n;JSON.stringify(lmtParseDeck(" + strconv.Quote(strconv.FormatInt(d.ID, 10)) + "))"
	var last string
	for {
		var out string
		if err := chromedp.Run(ctx, chromedp.Evaluate(expr, &out)); err == nil {
			last = out
			var s snapshot
			if json.Unmarshal([]byte(out), &s) == nil && len(s.Cards) > 0 && s.Totals.Avg != nil {
				return &s, nil
			}
		}
		select {
		case <-ctx.Done():
			if last == "" || last == "null" {
				var title string
				chromedp.Run(bctx, chromedp.Title(&title))
				return nil, fmt.Errorf("não achei os preços na página (título: %q). Pode ser o Cloudflare ou mudança no layout", title)
			}
			return nil, errors.New("a página não mostrou os preços a tempo")
		case <-time.After(time.Second):
		}
	}
}

func apiCall(cfg config, method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, cfg.Server+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+cfg.Key)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	// O plano free do Render dorme: o primeiro pedido pode levar até 1 minuto.
	res, err := (&http.Client{Timeout: 90 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("sem conexão com %s: %w", cfg.Server, err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	if res.StatusCode >= 300 {
		var e struct {
			Error string `json:"error"`
		}
		json.Unmarshal(data, &e)
		if e.Error == "" {
			e.Error = res.Status
		}
		return errors.New(e.Error)
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

// ---- configuração ----

func configDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "mtg-tracker")
	return dir, os.MkdirAll(dir, 0o700)
}

func loadConfig(dir string) config {
	var c config
	if b, err := os.ReadFile(filepath.Join(dir, "config.json")); err == nil {
		json.Unmarshal(b, &c)
	}
	return c
}

func saveConfig(dir string, c config) error {
	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(filepath.Join(dir, "config.json"), b, 0o600)
}

// ---- agendamento ----

func addSchedule() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	switch runtime.GOOS {
	case "windows":
		// conhost --headless evita a janela preta piscando a cada hora.
		tr := fmt.Sprintf(`conhost.exe --headless "%s" -agendado`, exe)
		out, err := exec.Command("schtasks", "/Create", "/F", "/SC", "HOURLY", "/TN", taskName, "/TR", tr).CombinedOutput()
		if err != nil {
			return fmt.Errorf("%v: %s", err, out)
		}
		fmt.Println("Agendado: roda de hora em hora enquanto o computador estiver ligado (Agendador de Tarefas, \"" + taskName + "\").")
	default:
		line := fmt.Sprintf(`0 * * * * "%s" -agendado`, exe)
		cur, _ := exec.Command("crontab", "-l").Output()
		if strings.Contains(string(cur), exe) {
			fmt.Println("Já estava agendado no crontab.")
			return nil
		}
		cmd := exec.Command("crontab", "-")
		cmd.Stdin = strings.NewReader(strings.TrimRight(string(cur), "\n") + "\n" + line + "\n")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%v: %s (adicione no crontab: %s)", err, out, line)
		}
		fmt.Println("Agendado no crontab: roda de hora em hora.")
	}
	return nil
}

func removeSchedule() error {
	switch runtime.GOOS {
	case "windows":
		out, err := exec.Command("schtasks", "/Delete", "/F", "/TN", taskName).CombinedOutput()
		if err != nil {
			return fmt.Errorf("%v: %s", err, out)
		}
	default:
		exe, _ := os.Executable()
		cur, _ := exec.Command("crontab", "-l").Output()
		var keep []string
		for _, l := range strings.Split(strings.TrimRight(string(cur), "\n"), "\n") {
			if l != "" && !strings.Contains(l, exe) {
				keep = append(keep, l)
			}
		}
		cmd := exec.Command("crontab", "-")
		cmd.Stdin = strings.NewReader(strings.Join(keep, "\n") + "\n")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("%v: %s", err, out)
		}
	}
	return nil
}

// ---- navegador ----

// findBrowser procura o Edge quando não há Chrome (o chromedp já procura o Chrome sozinho).
func findBrowser() string {
	var paths []string
	switch runtime.GOOS {
	case "windows":
		local := os.Getenv("LOCALAPPDATA")
		paths = []string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
			filepath.Join(local, `Google\Chrome\Application\chrome.exe`),
			`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
			`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
		}
	case "darwin":
		paths = []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
		}
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "" // deixa o chromedp procurar
}

// ---- utilidades ----

func deckLabel(d deck) string {
	if d.Name != "" {
		return d.Name
	}
	return "deck " + strconv.FormatInt(d.ID, 10)
}

func brl(c *int64) string {
	if c == nil {
		return "—"
	}
	s := strconv.FormatInt(*c, 10)
	for len(s) < 3 {
		s = "0" + s
	}
	ints, dec := s[:len(s)-2], s[len(s)-2:]
	var b strings.Builder
	for i, r := range ints {
		if i > 0 && (len(ints)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	return "R$ " + b.String() + "," + dec
}

var stdin = bufio.NewReader(os.Stdin)

func ask(prompt string) string {
	fmt.Print(prompt)
	s, _ := stdin.ReadString('\n')
	return strings.TrimSpace(s)
}

func yes(s string) bool {
	s = strings.ToLower(s)
	return s == "" || s == "s" || s == "sim" || s == "y"
}

func isTerminal() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// trimLog zera o log quando passa de 1 MB.
func trimLog(f *os.File) {
	if fi, err := f.Stat(); err == nil && fi.Size() > 1<<20 {
		f.Truncate(0)
	}
}

func fatal(err error) {
	log.Printf("erro: %v", err)
	if isTerminal() {
		ask("\nPressione Enter para sair.")
	}
	os.Exit(1)
}
