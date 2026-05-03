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

	// M2 — DAB:
	CreateIdfrTableIfNotExists(spec IdfrTableSpec) string
	CreateFocalTableIfNotExists(spec FocalTableSpec) string
	CreateDescriptorTableIfNotExists(spec DescriptorTableSpec) string
	CreateRelationshipTableIfNotExists(spec RelationshipTableSpec) string

	MergeIdfr(spec MergeIdfrSpec) string
	MergeFocal(spec MergeFocalSpec) string
	MergeDescriptor(spec MergeDescriptorSpec) string
	MergeRelationship(spec MergeRelationshipSpec) string

	RecomputeIdfrRowSt(spec RecomputeIdfrRowStSpec) string
	RecomputeDescriptorRowSt(spec RecomputeDescriptorRowStSpec) string
	RecomputeRelationshipRowSt(spec RecomputeRelationshipRowStSpec) string

	CreateOrReplaceGroupView(spec GroupViewSpec) string
	CreateOrReplaceEntityCurrentView(spec EntityCurrentViewSpec) string
}
