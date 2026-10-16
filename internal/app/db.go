package app

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"strings"

	_ "github.com/tursodatabase/libsql-client-go/libsql"
)

//go:embed schema.sql
var schemaSQL string

// OpenDB abre o Turso (libsql://...) ou um arquivo SQLite local (file:...).
// O driver SQLite local só entra no build com -tags dev (e nos testes), para o binário de produção não precisar de CGO.
func OpenDB(url, token string) (*sql.DB, error) {
	driver, dsn := "libsql", url
	if strings.HasPrefix(url, "file:") {
		driver = "sqlite3"
		dsn = url + sep(url) + "_busy_timeout=5000&_journal_mode=WAL"
	} else if token != "" {
		dsn = url + sep(url) + "authToken=" + token
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}
	if driver == "sqlite3" {
		db.SetMaxOpenConns(1)
	}
	return db, nil
}

func sep(url string) string {
	if strings.Contains(url, "?") {
		return "&"
	}
	return "?"
}

func Migrate(ctx context.Context, db *sql.DB) error {
	for _, stmt := range strings.Split(schemaSQL, ";") {
		if !hasSQL(stmt) {
			continue
		}
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migrate: %w\n%s", err, stmt)
		}
	}
	return nil
}

// hasSQL ignora trechos que só têm comentários ou espaços.
func hasSQL(stmt string) bool {
	for _, line := range strings.Split(stmt, "\n") {
		l := strings.TrimSpace(line)
		if l != "" && !strings.HasPrefix(l, "--") {
			return true
		}
	}
	return false
}

func nullPtr(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}
