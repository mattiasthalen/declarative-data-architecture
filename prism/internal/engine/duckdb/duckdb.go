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

// --- M2 (DAB) — panic-stubs until templates are wired in Phase 3 -----------

func (dialect) CreateIdfrTableIfNotExists(spec engine.IdfrTableSpec) string {
	s, err := tmpl.RenderCreateIdfr(spec)
	if err != nil {
		panic(err)
	}
	return s
}
func (dialect) CreateFocalTableIfNotExists(spec engine.FocalTableSpec) string {
	s, err := tmpl.RenderCreateFocal(spec)
	if err != nil {
		panic(err)
	}
	return s
}
func (dialect) CreateDescriptorTableIfNotExists(spec engine.DescriptorTableSpec) string {
	s, err := tmpl.RenderCreateDescriptor(spec)
	if err != nil {
		panic(err)
	}
	return s
}
func (dialect) CreateRelationshipTableIfNotExists(spec engine.RelationshipTableSpec) string {
	s, err := tmpl.RenderCreateRelationship(spec)
	if err != nil {
		panic(err)
	}
	return s
}
func (dialect) MergeIdfr(spec engine.MergeIdfrSpec) string {
	s, err := tmpl.RenderMergeIdfr(spec)
	if err != nil {
		panic(err)
	}
	return s
}
func (dialect) MergeFocal(spec engine.MergeFocalSpec) string {
	s, err := tmpl.RenderMergeFocal(spec)
	if err != nil {
		panic(err)
	}
	return s
}
func (dialect) MergeDescriptor(spec engine.MergeDescriptorSpec) string {
	s, err := tmpl.RenderMergeDescriptor(spec)
	if err != nil {
		panic(err)
	}
	return s
}
func (dialect) MergeRelationship(spec engine.MergeRelationshipSpec) string {
	s, err := tmpl.RenderMergeRelationship(spec)
	if err != nil {
		panic(err)
	}
	return s
}
func (dialect) RecomputeIdfrRowSt(spec engine.RecomputeIdfrRowStSpec) string {
	s, err := tmpl.RenderRecomputeIdfrRowSt(spec)
	if err != nil {
		panic(err)
	}
	return s
}
func (dialect) RecomputeDescriptorRowSt(spec engine.RecomputeDescriptorRowStSpec) string {
	s, err := tmpl.RenderRecomputeDescriptorRowSt(spec)
	if err != nil {
		panic(err)
	}
	return s
}
func (dialect) RecomputeRelationshipRowSt(spec engine.RecomputeRelationshipRowStSpec) string {
	s, err := tmpl.RenderRecomputeRelationshipRowSt(spec)
	if err != nil {
		panic(err)
	}
	return s
}
func (dialect) CreateOrReplaceGroupView(spec engine.GroupViewSpec) string {
	s, err := tmpl.RenderCreateGroupView(spec)
	if err != nil {
		panic(err)
	}
	return s
}
func (dialect) CreateOrReplaceEntityCurrentView(spec engine.EntityCurrentViewSpec) string {
	s, err := tmpl.RenderCreateEntityCurrentView(spec)
	if err != nil {
		panic(err)
	}
	return s
}
