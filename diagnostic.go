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

package markst

import (
	"fmt"
	"io"
	"strings"

	"znkr.io/markst/eval"
	"znkr.io/markst/syntax"
)

// Severity distinguishes a diagnostic that stopped a document from compiling
// from one that merely describes something questionable about a document that
// did.
type Severity int

const (
	// Warning describes a document that compiled: a label used twice, content
	// discarded where it can have no effect.
	Warning Severity = iota
	// Error describes a document that did not.
	Error
)

// String returns the severity's name, "warning" or "error".
func (s Severity) String() string {
	switch s {
	case Warning:
		return "warning"
	case Error:
		return "error"
	}
	return fmt.Sprintf("Severity(%d)", int(s))
}

// Diagnostic is one message about a document. [Diagnostic.Loc] carries both the
// byte offsets and the line and column they resolve to, so a caller holding a
// diagnostic can say where the problem is without the source.
//
// Realization works on content that has lost its link back to the syntax, so a
// diagnostic from that stage has nothing to point at and its Loc reports false
// from [syntax.Location.IsValid].
type Diagnostic struct {
	Severity Severity

	// Origin is the display name of the source Loc points into — the name
	// given to [CompileLibrary], or set by [WithName]. It is empty when the
	// host named neither.
	//
	// It is per-diagnostic rather than per-compile because one compile can
	// report on more than one source: a document that calls a library function
	// gets that library's failures, located in the library's own text.
	Origin string

	Loc   syntax.Location
	Msg   string
	Hints []Hint

	// Trace is the chain of calls that led here, outermost first, holding only
	// the calls that crossed from one source into another. It is empty for a
	// failure that happened in the source Loc already points at.
	Trace []Frame
}

// Hint is a suggestion attached to a [Diagnostic]. Loc points at what the
// suggestion is about, which is not always where the diagnostic points: a hint
// naming the callee of a failing call belongs on the callee, not on the
// argument that failed. It is invalid when the hint has nothing of its own to
// point at, in which case it is about the diagnostic's own location.
type Hint struct {
	Loc syntax.Location
	Msg string
}

// Frame is one call site on the path to a [Diagnostic]: where the call was
// written, in the source that wrote it.
type Frame struct {
	Origin string
	Loc    syntax.Location

	// Callee names the function being called, empty if it is anonymous.
	Callee string
}

// Error returns the message prefixed with the source name, line and column, so
// that printing a diagnostic still says where it happened. The parts are
// available separately through the fields.
func (d Diagnostic) Error() string {
	if pos := d.position(); pos != "" {
		return pos + ": " + d.Msg
	}
	return d.Msg
}

// position renders the "file:line:col" prefix, omitting either half the
// diagnostic does not have. It is empty when it has neither.
func (d Diagnostic) position() string {
	switch {
	case d.Origin != "" && d.Loc.IsValid():
		return d.Origin + ":" + d.Loc.Start.String()
	case d.Loc.IsValid():
		return d.Loc.Start.String()
	default:
		return d.Origin
	}
}

// DiagnosticList is the error [Compile] returns when a document fails to
// compile.
type DiagnosticList []Diagnostic

// Error returns every diagnostic in the list, one per line, so that printing a
// compile failure does not hide all but the first.
func (l DiagnosticList) Error() string {
	if len(l) == 0 {
		return "no errors"
	}
	var sb strings.Builder
	for i, d := range l {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(d.Error())
	}
	return sb.String()
}

// Unwrap returns the diagnostics as errors, so [errors.As] and [errors.Is] can
// reach an individual one.
func (l DiagnosticList) Unwrap() []error {
	r := make([]error, 0, len(l))
	for _, d := range l {
		r = append(r, d)
	}
	return r
}

// hintIndent is the indent hints are printed at, and hintContinuation is the
// indent their wrapped lines line up under.
const (
	hintIndent       = "  "
	hintContinuation = "        " // len(hintIndent + "hint: ")
)

// FormatDiagnostics writes diags to w in the conventional compiler form, one
// per line, with any hints and call sites indented beneath:
//
//	doc.wr:3:12: unknown variable: foo
//	doc.wr:7:1: missing argument: body
//	  hint: dates must be written as
//	        datetime(year: …)
//	lib.wr:5:9: invalid date: 2024-13-01
//	  note: called from doc.wr:1:2 in article
//	cannot convert integer to content
//
// Each diagnostic names its own source, so a compile that reached into a
// library reports the library's failures against the library's text; see
// [Diagnostic.Origin]. One with neither a name nor a location prints bare, as
// the last line above shows.
//
// Errors print without a label, as a compiler's do. Warnings are labeled, so
// that a list holding both stays readable.
func FormatDiagnostics(w io.Writer, diags []Diagnostic) error {
	for _, d := range diags {
		var sb strings.Builder
		if pos := d.position(); pos != "" {
			sb.WriteString(pos)
			sb.WriteString(": ")
		}
		if d.Severity != Error {
			sb.WriteString(d.Severity.String())
			sb.WriteString(": ")
		}
		sb.WriteString(d.Msg)
		sb.WriteByte('\n')
		for _, h := range d.Hints {
			for i, line := range strings.Split(h.Msg, "\n") {
				if i == 0 {
					sb.WriteString(hintIndent + "hint: ")
				} else {
					sb.WriteString(hintContinuation)
				}
				sb.WriteString(line)
				sb.WriteByte('\n')
			}
		}
		// Innermost last, so the notes read outward-in from the diagnostic
		// above them: the call nearest the failure sits nearest to it.
		for i := len(d.Trace) - 1; i >= 0; i-- {
			f := d.Trace[i]
			sb.WriteString(hintIndent + "note: called from ")
			sb.WriteString(Diagnostic{Origin: f.Origin, Loc: f.Loc}.position())
			if f.Callee != "" {
				sb.WriteString(" in ")
				sb.WriteString(f.Callee)
			}
			sb.WriteByte('\n')
		}
		if _, err := io.WriteString(w, sb.String()); err != nil {
			return err
		}
	}
	return nil
}

// diagnose resolves each evaluator diagnostic against its own origin, turning
// the offsets the pipeline works in into locations a caller can read. Each
// error carries the source its span indexes, so a run that touched several
// files resolves every diagnostic against the right one.
func diagnose(sev Severity, errs []eval.Error) []Diagnostic {
	if len(errs) == 0 {
		return nil
	}
	r := make([]Diagnostic, 0, len(errs))
	for _, e := range errs {
		var trace []Frame
		for _, f := range e.Trace {
			trace = append(trace, Frame{
				Origin: f.Origin.Name,
				Loc:    syntax.Locate(f.Origin.Source, f.Span),
				Callee: f.Callee,
			})
		}
		var hints []Hint
		for _, h := range e.Hints {
			loc := syntax.Location{}
			if h.Span != syntax.NoSpan {
				loc = syntax.Locate(e.Origin.Source, h.Span)
			}
			hints = append(hints, Hint{Loc: loc, Msg: h.Msg})
		}
		r = append(r, Diagnostic{
			Severity: sev,
			Origin:   e.Origin.Name,
			Loc:      syntax.Locate(e.Origin.Source, e.Span),
			Msg:      e.Msg,
			Hints:    hints,
			Trace:    trace,
		})
	}
	return r
}
