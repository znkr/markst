package graphemes

import (
	"unicode"
	"unicode/utf8"
)

// Graphemes iterates over Unicode grapheme clusters in s.
// This handles combining marks and ZWJ emoji sequences.
func Graphemes(s string) func(yield func(string) bool) {
	return func(yield func(string) bool) {
		for len(s) > 0 {
			_, size := utf8.DecodeRuneInString(s)
			end := size
			// Extend to include combining marks, ZWJ sequences, and variation selectors.
			for end < len(s) {
				r, sz := utf8.DecodeRuneInString(s[end:])
				if unicode.Is(unicode.M, r) || r == 0x200D || (r >= 0xFE00 && r <= 0xFE0F) {
					end += sz
					// After ZWJ, also consume the next character.
					if r == 0x200D && end < len(s) {
						_, sz2 := utf8.DecodeRuneInString(s[end:])
						end += sz2
					}
				} else {
					break
				}
			}
			if !yield(s[:end]) {
				return
			}
			s = s[end:]
		}
	}
}
