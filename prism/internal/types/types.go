// Package types defines prism's small canonical type system and its mapping to
// engine-specific SQL types. M1 supports DuckDB only.
package types

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Type represents a prism canonical type.
type Type struct {
	Name      string // STRING, INTEGER, BIGINT, BOOLEAN, DATE, TIMESTAMP, JSON, DECIMAL
	Precision int    // DECIMAL only
	Scale     int    // DECIMAL only
}

var basic = map[string]string{
	"STRING":    "VARCHAR",
	"INTEGER":   "INTEGER",
	"BIGINT":    "BIGINT",
	"BOOLEAN":   "BOOLEAN",
	"DATE":      "DATE",
	"TIMESTAMP": "TIMESTAMP",
	"JSON":      "JSON",
}

var decimalRe = regexp.MustCompile(`^DECIMAL\((\d+),\s*(\d+)\)$`)

// Parse parses a prism type literal (e.g. "STRING", "DECIMAL(18,4)").
func Parse(s string) (Type, error) {
	s = strings.TrimSpace(s)
	if sql, ok := basic[s]; ok {
		return Type{Name: s, Precision: 0, Scale: 0}, parseDuckOK(sql)
	}
	if strings.HasPrefix(s, "DECIMAL") {
		m := decimalRe.FindStringSubmatch(s)
		if m == nil {
			return Type{}, fmt.Errorf("DECIMAL requires precision and scale, e.g. DECIMAL(18,4); got %q", s)
		}
		p, _ := strconv.Atoi(m[1])
		sc, _ := strconv.Atoi(m[2])
		if p <= 0 || sc < 0 || sc > p {
			return Type{}, fmt.Errorf("DECIMAL precision/scale out of range: %q", s)
		}
		return Type{Name: "DECIMAL", Precision: p, Scale: sc}, nil
	}
	return Type{}, fmt.Errorf("unknown type %q (supported: STRING, INTEGER, BIGINT, BOOLEAN, DATE, TIMESTAMP, JSON, DECIMAL(p,s))", s)
}

// parseDuckOK is a noop sentinel; placeholder for future validation hooks.
func parseDuckOK(_ string) error { return nil }

// DuckDBType returns the DuckDB SQL type literal.
func (t Type) DuckDBType() string {
	if t.Name == "DECIMAL" {
		return fmt.Sprintf("DECIMAL(%d,%d)", t.Precision, t.Scale)
	}
	return basic[t.Name]
}
