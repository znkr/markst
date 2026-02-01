package scanner

import (
	"slices"
	"unicode/utf8"

	"znkr.io/writst/syntax"
)

type source struct {
	content  string
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
	var column uint32
	for i := range f.content[lineStart:] {
		if lineStart+uint32(i) >= offset {
			break
		}
		column++
	}
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
		_, chw := utf8.DecodeRuneInString(f.content[offset:])
		offset += uint32(chw)
	}
	return offset
}
