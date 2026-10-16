//go:build dev

package app

// Desenvolvimento local com arquivo SQLite: go run -tags dev ./cmd/server
import _ "github.com/mattn/go-sqlite3"
