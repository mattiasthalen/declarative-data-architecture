package naming

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// ToSnakeCase converts PascalCase / camelCase to snake_case.
// Sequences of uppercase letters are kept together (HTTPServer -> http_server).
func ToSnakeCase(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) {
			prev := runes[i-1]
			next := rune(0)
			if i+1 < len(runes) {
				next = runes[i+1]
			}
			// Insert underscore when transitioning from lower/digit to upper,
			// or from upper to upper-followed-by-lower (HTTPServer split).
			if unicode.IsLower(prev) || unicode.IsDigit(prev) {
				b.WriteRune('_')
			} else if unicode.IsUpper(prev) && next != 0 && unicode.IsLower(next) {
				b.WriteRune('_')
			}
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

var snakeRe = regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)*$`)

// ValidateSnakeCaseIdentifier returns an error if s is not a valid snake_case
// identifier (lowercase, digits, single underscores, leading letter).
func ValidateSnakeCaseIdentifier(s string) error {
	if s == "" {
		return fmt.Errorf("identifier is empty")
	}
	if !snakeRe.MatchString(s) {
		return fmt.Errorf("identifier %q is not snake_case (lowercase letters, digits, single underscores; must start with a letter)", s)
	}
	return nil
}
