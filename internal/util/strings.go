package util

import "strings"

// IsEmptyOrWhitespace returns true if the string is empty or contains only whitespace.
func IsEmptyOrWhitespace(s string) bool {
	return strings.TrimSpace(s) == ""
}
