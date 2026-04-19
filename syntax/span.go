package syntax

import "fmt"

// Span represents a range of text in the source document.
type Span struct {
	Start uint32 // Start offset (inclusive)
	End   uint32 // End offset (exclusive)
}

// Position represents a position in the source document, consisting of a line
// and column number.
type Position struct {
	Line   uint32 // Line number (1-based)
	Column uint32 // Column number (1-based)
}

func (p Position) String() string {
	return fmt.Sprintf("%d:%d", p.Line, p.Column)
}

// IsValid reports whether the position is valid (i.e., both line and column are
// greater than 0).
func (p *Position) IsValid() bool {
	return p.Line > 0 && p.Column > 0
}

// Source provides information about a source file.
type Source interface {
	// Position returns the position corresponding to the given offset in the
	// file.
	Position(offset uint32) Position

	// Offset returns the offset corresponding to the given position in the
	// file.
	Offset(pos Position) uint32
}
