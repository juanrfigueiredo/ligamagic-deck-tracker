package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/juanrfigueiredo/ligamagic-deck-tracker/internal/app"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	dbURL := env("TURSO_DATABASE_URL", "file:local.db")
	secret := os.Getenv("SECRET_KEY")
	if secret == "" {
		if !strings.HasPrefix(dbURL, "file:") {
			log.Fatal("SECRET_KEY é obrigatório em produção")
		}
		secret = "dev-secret"
	}

	db, err := app.OpenDB(dbURL, os.Getenv("TURSO_AUTH_TOKEN"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := app.Migrate(ctx, db); err != nil {
		log.Fatal(err)
	}

	mailer := app.NewResend(os.Getenv("RESEND_API_KEY"), env("MAIL_FROM", "LigaMagic Deck Tracker <noreply@jrfsoftware.com.br>"))
	a := app.New(db, mailer, secret, env("BASE_URL", "http://localhost:8080"))
	go a.Cleanup(ctx)

	srv := &http.Server{
		Addr:              ":" + env("PORT", "8080"),
		Handler:           a.Routes(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()
	log.Printf("ouvindo em %s", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
