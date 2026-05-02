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
