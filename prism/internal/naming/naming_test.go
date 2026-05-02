package naming

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToSnakeCase(t *testing.T) {
	cases := map[string]string{
		"Customer":         "customer",
		"SalesOrderHeader": "sales_order_header",
		"PurchaseOrderID":  "purchase_order_id",
		"already_snake":    "already_snake",
		"HTTPServer":       "http_server",
		"customerID":       "customer_id",
		"":                 "",
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			assert.Equal(t, want, ToSnakeCase(in))
		})
	}
}

func TestValidateSnakeCaseIdentifier(t *testing.T) {
	valid := []string{"customer", "sales_order_header", "x", "a1", "snake_case_123"}
	for _, s := range valid {
		t.Run("valid/"+s, func(t *testing.T) {
			require.NoError(t, ValidateSnakeCaseIdentifier(s))
		})
	}
	invalid := []string{"", "Customer", "_leading", "trailing_", "double__underscore", "1leading_digit", "with-dash", "with space"}
	for _, s := range invalid {
		t.Run("invalid/"+s, func(t *testing.T) {
			require.Error(t, ValidateSnakeCaseIdentifier(s))
		})
	}
}
