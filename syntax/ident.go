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

package syntax

import "unicode"

// IsIdentStart reports whether ch can start an identifier.
func IsIdentStart(ch rune) bool {
	// TODO: Use unicode XID_Start property?
	return unicode.IsLetter(ch) || ch == '_'
}

// IsIdentContinue reports whether ch can appear in an identifier after the
// first rune.
func IsIdentContinue(ch rune) bool {
	// TODO: Use unicode XID_Continue property?
	return unicode.IsLetter(ch) || unicode.IsNumber(ch) || ch == '-' || ch == '_'
}

// IsIdent reports whether s is spelled like an identifier, so that it can be
// written bare where markst expects a name — a dictionary key, a named
// argument. Keywords are not excluded: `let` is an identifier by this measure.
func IsIdent(s string) bool {
	for i, ch := range s {
		if i == 0 {
			if !IsIdentStart(ch) {
				return false
			}
			continue
		}
		if !IsIdentContinue(ch) {
			return false
		}
	}
	return s != ""
}
