// Package dab translates parsed DAB focal contracts into engine specs and
// SQL execution plans, and runs them against an engine.Engine.
package dab

import (
	"crypto/md5"
	"encoding/hex"
	"strings"
)

// IDFRSeparator joins multi-part primary_keys[] values into one canonical
// IDFR string. Chosen to avoid collisions with snake_case content and the
// SQL `||` operator characters appearing in raw data.
const IDFRSeparator = "||__||"

// TypeKeyHex returns the 32-char hex MD5 of "<entityID>:<attrOrRelID>".
// Used as the deterministic TYPE_KEY value for descriptors and relationships.
func TypeKeyHex(entityID, attrOrRelID string) string {
	h := md5.Sum([]byte(entityID + ":" + attrOrRelID))
	return hex.EncodeToString(h[:])
}

// CanonicalIDFRExpr produces a SQL expression that evaluates to the canonical
// IDFR string for one DAS row. Each primary_keys[] entry is wrapped in
// parentheses, cast to VARCHAR, and joined with IDFRSeparator. Single-element
// lists do not insert the separator.
func CanonicalIDFRExpr(primaryKeys []string) string {
	parts := make([]string, len(primaryKeys))
	for i, k := range primaryKeys {
		parts[i] = "CAST((" + k + ") AS VARCHAR)"
	}
	return strings.Join(parts, " || '"+IDFRSeparator+"' || ")
}
