package writst

import (
	"fmt"
	"io"
	"strings"

	"znkr.io/writst/eval"
	"znkr.io/writst/syntax"
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

func (s Severity) String() string {
	switch s {
	case Warning:
		return "warning"
	case Error:
		return "error"
	}
	return fmt.Sprintf("Severity(%d)", int(s))
}

// Diagnostic is a single message about a document, located in the source it
// was compiled from. It is self-describing: [Diagnostic.Loc] carries both the
// byte offsets and the line/column positions they resolve to, so a caller
// holding one needs neither the source bytes nor a [syntax.Source] to say
// where the problem is.
//
// A diagnostic produced during realization has nothing to point at — that
// stage works on content that has lost every link back to the syntax it came
// from — and its Loc reports false from [syntax.Location.IsValid].
type Diagnostic struct {
	Severity Severity
	Loc      syntax.Location
	Msg      string
	Hints    []string
}

// Error returns the diagnostic prefixed with its position, so a bare %v of a
// diagnostic still says where it happened. It exists so Diagnostic satisfies
// the standard library error interface; the parts are reachable individually
// through the fields.
func (d Diagnostic) Error() string {
	if !d.Loc.IsValid() {
		return d.Msg
	}
	return fmt.Sprintf("%s: %s", d.Loc.Start, d.Msg)
}

// DiagnosticList is the error [Compile] returns when a document fails to
// compile. Its Error method names every diagnostic in the list, so the default
// formatting of a compile failure does not hide all but the first.
type DiagnosticList []Diagnostic

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
// per line, with any hints indented beneath:
//
//	doc.wr:3:12: unknown variable: foo
//	doc.wr:7:1: missing argument: body
//	  hint: dates must be written as
//	        datetime(year: …)
//	doc.wr: cannot convert integer to content
//
// name is the document's display name — writst has no file concept of its own,
// so the caller supplies it here rather than storing it on each diagnostic. An
// empty name omits the prefix, and a diagnostic with no location omits the
// line:col, as the last line above shows.
//
// Errors print bare, as a compiler's do; a warning is labelled, so a list
// holding both stays readable.
func FormatDiagnostics(w io.Writer, name string, diags []Diagnostic) error {
	for _, d := range diags {
		var sb strings.Builder
		sb.WriteString(name)
		if d.Loc.IsValid() {
			if name != "" {
				sb.WriteByte(':')
			}
			sb.WriteString(d.Loc.Start.String())
		}
		if sb.Len() > 0 {
			sb.WriteString(": ")
		}
		if d.Severity != Error {
			sb.WriteString(d.Severity.String())
			sb.WriteString(": ")
		}
		sb.WriteString(d.Msg)
		sb.WriteByte('\n')
		for _, h := range d.Hints {
			for i, line := range strings.Split(h, "\n") {
				if i == 0 {
					sb.WriteString(hintIndent + "hint: ")
				} else {
					sb.WriteString(hintContinuation)
				}
				sb.WriteString(line)
				sb.WriteByte('\n')
			}
		}
		if _, err := io.WriteString(w, sb.String()); err != nil {
			return err
		}
	}
	return nil
}

// diagnose resolves each evaluator diagnostic's span against src, turning the
// offsets the pipeline works in into locations a caller can read.
func diagnose(src syntax.Source, sev Severity, errs []eval.Error) []Diagnostic {
	if len(errs) == 0 {
		return nil
	}
	r := make([]Diagnostic, 0, len(errs))
	for _, e := range errs {
		r = append(r, Diagnostic{
			Severity: sev,
			Loc:      syntax.Locate(src, e.Span),
			Msg:      e.Msg,
			Hints:    e.Hints,
		})
	}
	return r
}
