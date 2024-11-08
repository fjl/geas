package ast

import "unicode"

// IsGlobal returns true when 'name' is a global identifier.
func IsGlobal(name string) bool {
	return len(name) > 0 && unicode.IsUpper([]rune(name)[0])
}
