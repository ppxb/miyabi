package domain

import (
	"strings"
	"unicode"
)

// NormalizeActorName lowercases and removes all whitespace so provider-specific
// spellings of the same actor collapse to one identity.
func NormalizeActorName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, name)
}
