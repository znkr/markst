// Copyright 2026 Florian Zenker (flo@znkr.io)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package graphemes splits text into what a reader sees as one character.
package graphemes

import (
	"unicode"
	"unicode/utf8"
)

// All iterates the grapheme clusters of s, so a base letter and its combining
// marks, or a ZWJ emoji sequence, come out as one string.
func All(s string) func(yield func(string) bool) {
	return func(yield func(string) bool) {
		for len(s) > 0 {
			g, w := Decode(s)
			if !yield(g) {
				return
			}
			s = s[w:]
		}
	}
}

// Decode returns the first grapheme cluster of s and how many bytes it took.
// For an empty s it returns the replacement character and a width of 0.
func Decode(s string) (grapheme string, width int) {
	if len(s) == 0 {
		return string(utf8.RuneError), 0
	}
	_, w := utf8.DecodeRuneInString(s)
	width += w
	// Extend to include combining marks, ZWJ sequences, and variation selectors.
	for width < len(s) {
		r, sz := utf8.DecodeRuneInString(s[width:])
		if unicode.Is(unicode.M, r) || r == 0x200D || (r >= 0xFE00 && r <= 0xFE0F) {
			width += sz
			// After ZWJ, also consume the next character.
			if r == 0x200D && width < len(s) {
				_, sz2 := utf8.DecodeRuneInString(s[width:])
				width += sz2
			}
		} else {
			break
		}
	}
	return s[:width], width
}
