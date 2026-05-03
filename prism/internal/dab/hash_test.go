package dab_test

import (
	"crypto/md5"
	"encoding/hex"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prism-data/prism/internal/dab"
)

func TestTypeKeyHex_DescriptorType(t *testing.T) {
	got := dab.TypeKeyHex("CUSTOMER", "CUSTOMER_LIFETIME_VALUE")
	want := md5sum("CUSTOMER:CUSTOMER_LIFETIME_VALUE")
	require.Equal(t, want, got)
	require.Len(t, got, 32)
}

func TestTypeKeyHex_RelationshipType(t *testing.T) {
	got := dab.TypeKeyHex("CUSTOMER", "CUSTOMER_PLACES_ORDER")
	want := md5sum("CUSTOMER:CUSTOMER_PLACES_ORDER")
	require.Equal(t, want, got)
}

func TestCanonicalIDFRExpr_SingleKey(t *testing.T) {
	expr := dab.CanonicalIDFRExpr([]string{"customer_id"})
	require.Equal(t, "CAST((customer_id) AS VARCHAR)", expr)
}

func TestCanonicalIDFRExpr_MultiKey(t *testing.T) {
	expr := dab.CanonicalIDFRExpr([]string{"order_id", "line_no"})
	require.Equal(t, "CAST((order_id) AS VARCHAR) || '||__||' || CAST((line_no) AS VARCHAR)", expr)
}

func md5sum(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}
