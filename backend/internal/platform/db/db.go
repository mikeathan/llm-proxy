// Package db provides the shared SQLite Provider interface and a single
// point of configuration for the database connection (DSN, WAL mode, limits).
//
// Ledger, memory, and orchestrator each accept a Provider in their constructor
// and call .DB() once to store a *sql.DB reference for internal use.
package db

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

type Provider interface {
	DB() *sql.DB
}

type provider struct {
	db *sql.DB
}

func (p *provider) DB() *sql.DB { return p.db }

func Open(path string) (Provider, error) {
	dsn := path + "?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&_foreign_keys=ON"
	database, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("db open: %w", err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)
	return &provider{db: database}, nil
}
