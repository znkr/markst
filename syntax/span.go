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

import "fmt"

// Span is a range of bytes in a source document.
type Span struct {
	Start uint32 // Start offset (inclusive)
	End   uint32 // End offset (exclusive)
}

// NoSpan marks a diagnostic that cannot be attributed to a source location.
// Realization is the stage that needs it: it works on content that has lost
// every link back to the syntax it came from, so a failure there has nothing
// to point at. [Locate] resolves it to the zero [Location].
var NoSpan = Span{Start: ^uint32(0), End: ^uint32(0)}

// Position is a line and column in a source document, both counted from 1.
type Position struct {
	Line   uint32 // Line number (1-based)
	Column uint32 // Column number (1-based)
}

// String returns the position as "line:column".
func (p Position) String() string {
	return fmt.Sprintf("%d:%d", p.Line, p.Column)
}

// IsValid reports whether the position names a place in a source, which the
// zero Position does not.
func (p Position) IsValid() bool {
	return p.Line > 0 && p.Column > 0
}

// Source converts between byte offsets and line/column positions in one
// source document.
type Source interface {
	// Position returns the position corresponding to the given offset in the
	// file.
	Position(offset uint32) Position

	// Offset returns the offset corresponding to the given position in the
	// file.
	Offset(pos Position) uint32
}

// Origin says which source a [Span] belongs to: what resolves its offsets, and
// what to call it in a diagnostic. Documents and libraries are alike here,
// except that a library always has a name and a one-off document may not.
//
// The zero Origin is usable: [Locate] resolves any span against it to the zero
// [Location], the same result [NoSpan] gives.
type Origin struct {
	// Name is the display name diagnostics about this source are reported
	// under, e.g. "lib.mst". Empty when the host supplied none.
	Name string

	// Source resolves offsets within this origin to line/column positions.
	Source Source
}

// Frame is one call site on the path to a diagnostic: where the call was
// written, in the source that wrote it. Only calls crossing from one [Origin]
// to another get a frame, so a failure inside a library says which document
// called it, without the rest of a stack trace.
type Frame struct {
	// Origin is the source Span points into — the caller's, not the callee's.
	Origin Origin

	// Span covers the call expression.
	Span Span

	// Callee names the function being called, empty if it is anonymous.
	Callee string
}

// Location is a [Span] with the line and column positions its offsets resolve
// to. Carrying both means a diagnostic holding one can say where it happened
// without the source or a [Source] to hand.
type Location struct {
	Span  Span     // byte offsets
	Start Position // position of Span.Start
	End   Position // position of Span.End
}

// IsValid reports whether the location resolves to a place in the source.
// It is false for a location built from [NoSpan].
func (l Location) IsValid() bool { return l.Start.IsValid() }

// String returns the location as "line:column", or as "line:column-line:column"
// when it spans more than a point. An invalid location prints "<no location>".
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
