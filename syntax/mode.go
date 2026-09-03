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

// Mode is one of the three ways Markst source can be read: as markup, as math,
// or as code. The same characters tokenize differently in each, so the parser
// switches the scanner's mode as it enters and leaves each construct.
type Mode int

const (
	// ModeMarkup is the default mode for text and markup constructs.
	// Whitespace in markup mode is limited to space (U+0020), tab (U+0009),
	// and newlines (LF, CR).
	ModeMarkup Mode = iota
	// ModeMath is active inside math equations ($ ... $).
	ModeMath
	// ModeCode is active inside code blocks and after #.
	// Whitespace in code mode includes all Unicode whitespace characters
	// (as defined by unicode.IsSpace).
	ModeCode
)
