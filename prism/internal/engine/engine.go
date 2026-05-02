// Package engine defines prism's storage-engine abstraction. M1 implements
// DuckDB only (see internal/engine/duckdb). Future engines (Postgres,
// BigQuery, Databricks) plug in here behind the same interface — see ADR-004.
package engine

import "context"

// Engine represents a connected warehouse engine.
type Engine interface {
	Close() error
	Exec(ctx context.Context, sql string) error
	Dialect() Dialect
}

// Dialect produces engine-specific SQL strings from spec structs. Pure
// rendering — no IO.
type Dialect interface {
	QuoteIdent(name string) string
	Schema(name string) string

	CreateSchemaIfNotExists(schema string) string
	CreateHistorizedTableIfNotExists(spec HistorizedTableSpec) string
	AppendIntoHistorized(spec HistorizedAppendSpec) string
	CreateOrReplaceCurrentView(spec CurrentViewSpec) string
}
