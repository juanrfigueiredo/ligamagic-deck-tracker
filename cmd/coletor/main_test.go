package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"

	"github.com/chromedp/chromedp"

	"github.com/juanrfigueiredo/ligamagic-deck-tracker/extension"
)

// Abre testdata/deck.html num Chrome oculto e lê com o mesmo parse.js da extensão.
func TestReadDeck(t *testing.T) {
	path := findBrowser()
	if path == "" {
		for _, n := range []string{"google-chrome", "chromium", "chromium-browser"} {
			if p, err := exec.LookPath(n); err == nil {
				path = p
				break
			}
		}
	}
	if path == "" {
		t.Skip("sem Chrome")
	}
	srv := httptest.NewServer(http.FileServer(http.Dir("testdata")))
	defer srv.Close()

	actx, cancel := chromedp.NewExecAllocator(context.Background(), append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(path), chromedp.NoSandbox)...)
	defer cancel()
	bctx, cancel := chromedp.NewContext(actx)
	defer cancel()

	js, _ := extension.Files.ReadFile("parse.js")
	s, err := readDeck(bctx, string(js), deck{ID: 777, URL: srv.URL + "/deck.html"})
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "Zurgo Aggro" || len(s.Cards) != 3 || *s.Totals.Avg != 177358 {
		t.Fatalf("snapshot: %+v", s)
	}
	if c := string(s.Cards[0]); !strings.Contains(c, `"cmc":4`) || !strings.Contains(c, `"mana":"2RR"`) || !strings.Contains(c, `"type":"Comandante"`) {
		t.Fatalf("carta 1: %s", c)
	}
	if c := string(s.Cards[1]); !strings.Contains(c, `"min":null`) || !strings.Contains(c, `"qty":4`) || !strings.Contains(c, `"cmc":0`) || !strings.Contains(c, `"type":"Terrenos"`) {
		t.Fatalf("carta 2: %s", c)
	}
	if c := string(s.Cards[2]); !strings.Contains(c, `"board":"side"`) || !strings.Contains(c, `"cmc":null`) {
		t.Fatalf("sideboard: %s", c)
	}
}

func TestBRL(t *testing.T) {
	for in, want := range map[int64]string{5: "R$ 0,05", 177358: "R$ 1.773,58", 123456789: "R$ 1.234.567,89"} {
		if got := brl(&in); got != want {
			t.Errorf("brl(%d) = %s, quero %s", in, got, want)
		}
	}
}
