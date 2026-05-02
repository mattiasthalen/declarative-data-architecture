// prism/internal/drift/drift.go
// Package drift runs lightweight contract assertions against materialized
// historized tables. M1 implements the NULL-in-REQUIRED check (see ADR-003).
package drift

import (
	"context"
	"fmt"

	"github.com/prism-data/prism/internal/engine"
	"github.com/prism-data/prism/internal/engine/duckdb"
)

// Result describes a single drift finding.
type Result struct {
	Column    string
	NullCount int
}

// DetectNullsInRequired returns one Result per REQUIRED column with at least
// one NULL value in the given historized table.
func DetectNullsInRequired(
	ctx context.Context, e *duckdb.Engine,
	schema, table string, cols []engine.Column,
) ([]Result, error) {
	var results []Result
	for _, c := range cols {
		if !c.NotNull {
			continue
		}
		q := fmt.Sprintf(
			`SELECT count(*) FROM "%s"."%s" WHERE "%s" IS NULL`,
			schema, table, c.TargetName,
		)
		rows, err := e.Query(ctx, q)
		if err != nil {
			return nil, err
		}
		var n int
		if rows.Next() {
			if err := rows.Scan(&n); err != nil {
				rows.Close()
				return nil, err
			}
		}
		rows.Close()
		if n > 0 {
			results = append(results, Result{Column: c.TargetName, NullCount: n})
		}
	}
	return results, nil
}
