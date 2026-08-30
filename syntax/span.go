package syntax

import "fmt"

// Span represents a range of text in the source document.
type Span struct {
	Start uint32 // Start offset (inclusive)
	End   uint32 // End offset (exclusive)
}

// NoSpan marks a diagnostic that cannot be attributed to a source location.
// Realization is the stage that needs it: it works on content that has lost
// every link back to the syntax it came from, so a failure there has nothing
// to point at. [Locate] resolves it to the zero [Location].
var NoSpan = Span{Start: ^uint32(0), End: ^uint32(0)}

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
func (p Position) IsValid() bool {
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

// Location is a [Span] resolved against a [Source]: the byte offsets together
// with the line/column positions they correspond to. It exists so a diagnostic
// can be self-describing — a caller holding one needs neither the [Source] nor
// the source bytes to say where the problem is.
type Location struct {
	Span  Span     // byte offsets
	Start Position // position of Span.Start
	End   Position // position of Span.End
}

// IsValid reports whether the location resolves to a place in the source.
// It is false for a location built from [NoSpan].
func (l Location) IsValid() bool { return l.Start.IsValid() }

func (l Location) String() string {
	if !l.IsValid() {
		return "<no location>"
	}
	if l.Start == l.End {
		return l.Start.String()
	}
	return fmt.Sprintf("%s-%s", l.Start, l.End)
}

// Locate resolves span against src. A [NoSpan] span, or a nil src, yields the
// zero [Location], for which [Location.IsValid] reports false.
func Locate(src Source, span Span) Location {
	if src == nil || span == NoSpan {
		return Location{}
	}
	return Location{
		Span:  span,
		Start: src.Position(span.Start),
		End:   src.Position(span.End),
	}
}
