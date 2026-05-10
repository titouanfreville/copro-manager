// Package text holds tiny string helpers shared across domain code.
// Kept minimal on purpose — most string handling belongs in the
// `strings` stdlib package; only the cases where the stdlib is
// surprising (UTF-8 byte cuts) live here.
package text

import "unicode/utf8"

// Truncate caps a string at `maxBytes` without splitting a multi-byte
// rune. Plain `s[:maxBytes]` would corrupt UTF-8 when the byte cut
// lands inside a multi-byte sequence (every accented French char is
// 2 bytes, so `Société` truncated at 5 lands inside the second `é`).
// We walk back to the nearest rune-start.
//
// Returns "" for non-positive `maxBytes`. Returns the input unchanged
// when shorter than the cap.
func Truncate(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}
