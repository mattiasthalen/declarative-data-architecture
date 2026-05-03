// Package duckdb renders prism's SQL templates against the DuckDB dialect.
package duckdb

import (
	"bytes"
	"embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/prism-data/prism/internal/engine"
)

//go:embed *.sql.tmpl
var tmplFS embed.FS

var funcs = template.FuncMap{
	"quote": quoteIdent,
}

// quoteIdent wraps an identifier in DuckDB's double-quote escaping.
// Example: quoteIdent("das__adventure_works") -> `"das__adventure_works"`.
func quoteIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func render(name string, data any) (string, error) {
	t, err := template.New(name).Funcs(funcs).ParseFS(tmplFS, name)
	if err != nil {
		return "", fmt.Errorf("parse template %s: %w", name, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("execute template %s: %w", name, err)
	}
	return strings.TrimSpace(buf.String()), nil
}

func RenderCreateSchema(schema string) (string, error) {
	return render("create_schema.sql.tmpl", struct{ Schema string }{schema})
}

func RenderCreateHistorized(spec engine.HistorizedTableSpec) (string, error) {
	return render("create_historized.sql.tmpl", spec)
}

func RenderAppendHistorized(spec engine.HistorizedAppendSpec) (string, error) {
	return render("append_historized.sql.tmpl", spec)
}

func RenderCreateCurrentView(spec engine.CurrentViewSpec) (string, error) {
	return render("create_current_view.sql.tmpl", spec)
}

// --- M2 (DAB) renderers ----------------------------------------------------

func RenderCreateIdfr(spec engine.IdfrTableSpec) (string, error) {
	return render("dab_create_idfr.sql.tmpl", struct {
		Schema, Table, KeyCol, IdfrCol string
	}{
		spec.Schema,
		spec.Entity + "__idfr",
		spec.Entity + "_key",
		spec.Entity + "_idfr",
	})
}

func RenderCreateFocal(spec engine.FocalTableSpec) (string, error) {
	return render("dab_create_focal.sql.tmpl", struct {
		Schema, Table, KeyCol string
	}{
		spec.Schema,
		spec.Entity,
		spec.Entity + "_key",
	})
}

func RenderCreateDescriptor(spec engine.DescriptorTableSpec) (string, error) {
	return render("dab_create_descriptor.sql.tmpl", struct {
		Schema, Table, KeyCol string
	}{
		spec.Schema,
		spec.Entity + "__descriptor",
		spec.Entity + "_key",
	})
}

func RenderCreateRelationship(spec engine.RelationshipTableSpec) (string, error) {
	return render("dab_create_relationship.sql.tmpl", struct {
		Schema, Table, KeyCol, RelatedCol string
	}{
		spec.Schema,
		spec.Entity + "__" + spec.Related + spec.Suffix + "__rel",
		spec.Entity + "_key",
		spec.Related + "_key",
	})
}

func RenderMergeIdfr(spec engine.MergeIdfrSpec) (string, error) {
	return render("dab_merge_idfr.sql.tmpl", struct {
		Schema, Table, KeyCol, IdfrCol, MappingGroup, InstRowKey, SourceCTE string
	}{
		spec.Schema,
		spec.Entity + "__idfr",
		spec.Entity + "_key",
		spec.Entity + "_idfr",
		spec.MappingGroup, spec.InstRowKey, spec.SourceCTE,
	})
}

func RenderMergeFocal(spec engine.MergeFocalSpec) (string, error) {
	return render("dab_merge_focal.sql.tmpl", struct {
		Schema, Table, IdfrTable, KeyCol string
	}{
		spec.Schema,
		spec.Entity,
		spec.Entity + "__idfr",
		spec.Entity + "_key",
	})
}

func RenderMergeDescriptor(spec engine.MergeDescriptorSpec) (string, error) {
	return render("dab_merge_descriptor.sql.tmpl", struct {
		Schema, Table, KeyCol, MappingGroup, InstRowKey, SourceCTE string
	}{
		spec.Schema,
		spec.Entity + "__descriptor",
		spec.Entity + "_key",
		spec.MappingGroup, spec.InstRowKey, spec.SourceCTE,
	})
}

func RenderMergeRelationship(spec engine.MergeRelationshipSpec) (string, error) {
	return render("dab_merge_relationship.sql.tmpl", struct {
		Schema, Table, KeyCol, RelatedCol, MappingGroup, InstRowKey, SourceCTE string
	}{
		spec.Schema,
		spec.Entity + "__" + spec.Related + spec.Suffix + "__rel",
		spec.Entity + "_key",
		spec.Related + "_key",
		spec.MappingGroup, spec.InstRowKey, spec.SourceCTE,
	})
}

func RenderRecomputeIdfrRowSt(spec engine.RecomputeIdfrRowStSpec) (string, error) {
	return render("dab_recompute_idfr.sql.tmpl", struct {
		Schema, Table, KeyCol, IdfrCol string
	}{
		spec.Schema,
		spec.Entity + "__idfr",
		spec.Entity + "_key",
		spec.Entity + "_idfr",
	})
}

func RenderRecomputeDescriptorRowSt(spec engine.RecomputeDescriptorRowStSpec) (string, error) {
	return render("dab_recompute_descriptor.sql.tmpl", struct {
		Schema, Table, KeyCol string
	}{
		spec.Schema,
		spec.Entity + "__descriptor",
		spec.Entity + "_key",
	})
}

func RenderRecomputeRelationshipRowSt(spec engine.RecomputeRelationshipRowStSpec) (string, error) {
	return render("dab_recompute_relationship.sql.tmpl", struct {
		Schema, Table, KeyCol, RelatedCol string
	}{
		spec.Schema,
		spec.Entity + "__" + spec.Related + spec.Suffix + "__rel",
		spec.Entity + "_key",
		spec.Related + "_key",
	})
}
