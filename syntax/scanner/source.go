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

package scanner

import (
	"slices"
	"unicode/utf8"

	"znkr.io/markst/syntax"
)

// source implements [syntax.Source] using the newline offsets collected during
// scanning. It maps between byte offsets and line/column positions using
// binary search over the newline table.
type source struct {
	content  []byte
	newlines []uint32
}

var _ syntax.Source = (*source)(nil)

// Position implements [syntax.Source].
func (f *source) Position(offset uint32) syntax.Position {
	line, ok := slices.BinarySearch(f.newlines, offset)
	if ok {
		line++
	}
	lineStart := uint32(0)
	if line > 0 {
		lineStart = f.newlines[line-1]
	}
	end := offset
	if end > uint32(len(f.content)) {
		end = uint32(len(f.content))
	}
	column := uint32(utf8.RuneCount(f.content[lineStart:end]))
	return syntax.Position{Line: uint32(line + 1), Column: column + 1}
}

// Offset implements [syntax.Source].
func (f *source) Offset(pos syntax.Position) uint32 {
	if pos.Line <= 0 || pos.Column <= 0 {
		return 0
	}
	offset := uint32(0)
	if pos.Line > 1 {
		offset = f.newlines[pos.Line-2]
	}
	for range pos.Column - 1 {
		_, chw := utf8.DecodeRune(f.content[offset:])
		offset += uint32(chw)
	}
	return offset
}
