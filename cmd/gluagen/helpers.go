package main

import (
	"strings"
	"unicode"
)

// toSnakeCase converts CamelCase to snake_case.
// "NewPlayer" → "new_player", "HTTPClient" → "http_client"
func toSnakeCase(s string) string {
	if s == "" {
		return ""
	}
	var result strings.Builder
	runes := []rune(s)
	for i, r := range runes {
		if unicode.IsUpper(r) {
			if i > 0 {
				prev := runes[i-1]
				// Insert underscore before an uppercase letter if:
				// 1. previous char is lowercase, OR
				// 2. next char is lowercase (handles "HTTPClient" → "http_client")
				if unicode.IsLower(prev) {
					result.WriteByte('_')
				} else if i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
					result.WriteByte('_')
				}
			}
			result.WriteRune(unicode.ToLower(r))
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}
