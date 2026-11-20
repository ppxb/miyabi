package domain

import (
	"crypto/sha1"
	"encoding/hex"
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

// ActorNameKey returns the stable key stored on Actor.NameKey for a raw name.
func ActorNameKey(name string) string {
	sum := sha1.Sum([]byte(NormalizeActorName(name)))
	return hex.EncodeToString(sum[:])
}
