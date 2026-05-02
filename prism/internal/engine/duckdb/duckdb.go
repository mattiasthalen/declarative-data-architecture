// Package duckdb is the DuckDB implementation of the engine.Engine interface.
package duckdb

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/marcboeker/go-duckdb/v2"

	"github.com/prism-data/prism/internal/engine"
	tmpl "github.com/prism-data/prism/internal/tmpl/duckdb"
)

type Engine struct {
	db *sql.DB
}

// Open returns a connected DuckDB engine. Path may be a filesystem path or
// `:memory:` for an ephemeral DB.
func Open(path string) (*Engine, error) {
	db, err := sql.Open("duckdb", path)
	if err != nil {
		return nil, fmt.Errorf("open duckdb %s: %w", path, err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping duckdb %s: %w", path, err)
	}
	return &Engine{db: db}, nil
}

func (e *Engine) Close() error { return e.db.Close() }

func (e *Engine) Exec(ctx context.Context, sql string) error {
	_, err := e.db.ExecContext(ctx, sql)
	if err != nil {
		return fmt.Errorf("exec: %w\n--- sql ---\n%s\n", err, sql)
	}
	return nil
}

// Query is exposed for tests / discovery / future inspection commands.
func (e *Engine) Query(ctx context.Context, q string) (*sql.Rows, error) {
	return e.db.QueryContext(ctx, q)
}

func (e *Engine) Dialect() engine.Dialect { return dialect{} }

type dialect struct{}

func (dialect) QuoteIdent(name string) string { return `"` + name + `"` }
func (dialect) Schema(name string) string     { return name }

func (dialect) CreateSchemaIfNotExists(schema string) string {
	s, err := tmpl.RenderCreateSchema(schema)
	if err != nil {
		panic(err) // template errors are programmer bugs
	}
	return s
}

func (dialect) CreateHistorizedTableIfNotExists(spec engine.HistorizedTableSpec) string {
	s, err := tmpl.RenderCreateHistorized(spec)
	if err != nil {
		panic(err)
	}
	return s
}

func (dialect) AppendIntoHistorized(spec engine.HistorizedAppendSpec) string {
	s, err := tmpl.RenderAppendHistorized(spec)
	if err != nil {
		panic(err)
	}
	return s
}

func (dialect) CreateOrReplaceCurrentView(spec engine.CurrentViewSpec) string {
	s, err := tmpl.RenderCreateCurrentView(spec)
	if err != nil {
		panic(err)
	}
	return s
}
