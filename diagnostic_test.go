package writst_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"znkr.io/writst"
	"znkr.io/writst/syntax"
)

// threeErrors is a document with one unknown variable per construct, spread
// over distinct lines and columns so a position mix-up shows up.
const threeErrors = "#foo\n\n#bar(1)\n\n#let x = baz\n"

// TestCompileDiagnosticsResolvePositions is the point of the whole exercise: a
// caller holding only Compile's return values can say where each problem is,
// without going back to the source bytes.
func TestCompileDiagnosticsResolvePositions(t *testing.T) {
	_, _, err := writst.Compile([]byte(threeErrors))
	if err == nil {
		t.Fatal("Compile() = nil error, want three errors")
	}
	var diags writst.DiagnosticList
	if !errors.As(err, &diags) {
		t.Fatalf("Compile() error is %T, want writst.DiagnosticList", err)
	}

	want := []struct {
		start syntax.Position
		end   syntax.Position
		msg   string
		text  string // the source the span covers
	}{
		{syntax.Position{Line: 1, Column: 2}, syntax.Position{Line: 1, Column: 5}, "unknown variable: foo", "foo"},
		{syntax.Position{Line: 3, Column: 2}, syntax.Position{Line: 3, Column: 5}, "unknown variable: bar", "bar"},
		{syntax.Position{Line: 5, Column: 10}, syntax.Position{Line: 5, Column: 13}, "unknown variable: baz", "baz"},
	}
	if len(diags) != len(want) {
		t.Fatalf("got %d diagnostics, want %d:\n%v", len(diags), len(want), err)
	}
	for i, w := range want {
		got := diags[i]
		if !got.Loc.IsValid() {
			t.Errorf("diags[%d].Loc is not valid", i)
		}
		if got.Loc.Start != w.start || got.Loc.End != w.end {
			t.Errorf("diags[%d].Loc = %v, want %v-%v", i, got.Loc, w.start, w.end)
		}
		if got.Msg != w.msg {
			t.Errorf("diags[%d].Msg = %q, want %q", i, got.Msg, w.msg)
		}
		if got.Severity != writst.Error {
			t.Errorf("diags[%d].Severity = %v, want %v", i, got.Severity, writst.Error)
		}
		// The offsets must still agree with the positions they resolved from.
		if text := threeErrors[got.Loc.Span.Start:got.Loc.Span.End]; text != w.text {
			t.Errorf("diags[%d].Loc.Span covers %q, want %q", i, text, w.text)
		}
	}
}

// TestDiagnosticListReportsEveryDiagnostic guards the defect that started
// this: an Error method that returned only the first message, so the default
// %v of a compile failure silently discarded the rest.
func TestDiagnosticListReportsEveryDiagnostic(t *testing.T) {
	_, _, err := writst.Compile([]byte(threeErrors))
	if err == nil {
		t.Fatal("Compile() = nil error, want three errors")
	}
	got := fmt.Sprintf("%v", err)
	for _, want := range []string{"foo", "bar", "baz"} {
		if !strings.Contains(got, want) {
			t.Errorf("%%v of the error does not mention %q:\n%s", want, got)
		}
	}
	if n := strings.Count(got, "\n") + 1; n != 3 {
		t.Errorf("%%v of the error has %d lines, want 3:\n%s", n, got)
	}
}

// TestDiagnosticListEmpty covers the other half of the same defect: the old
// Error method indexed [0] unguarded.
func TestDiagnosticListEmpty(t *testing.T) {
	var empty writst.DiagnosticList
	got := empty.Error() // must not panic
	if got == "" {
		t.Error("empty DiagnosticList.Error() = \"\", want a message")
	}
	if u := empty.Unwrap(); len(u) != 0 {
		t.Errorf("empty DiagnosticList.Unwrap() = %v, want empty", u)
	}
}

// TestDiagnosticAtEndOfInput guards the phantom trailing line: for input that
// does not end in a line terminator, a diagnostic anchored at len(src) used to
// resolve to a line that does not exist.
func TestDiagnosticAtEndOfInput(t *testing.T) {
	for _, tt := range []struct {
		name    string
		src     string
		lines   uint32 // lines the source actually has
		wantEnd uint32 // offset the diagnostic is anchored at
	}{
		{"no_trailing_newline", "#let x =", 1, 8},
		{"trailing_newline", "#let x =\n", 2, 8},
		{"multiline_no_trailing_newline", "a\n\n#let x =", 3, 11},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := writst.Compile([]byte(tt.src))
			var diags writst.DiagnosticList
			if !errors.As(err, &diags) || len(diags) == 0 {
				t.Fatalf("Compile(%q) = %v, want at least one error", tt.src, err)
			}
			for i, d := range diags {
				if d.Loc.Start.Line > tt.lines {
					t.Errorf("diags[%d].Loc.Start = %v, past the last line %d", i, d.Loc.Start, tt.lines)
				}
				if d.Loc.End.Line > tt.lines {
					t.Errorf("diags[%d].Loc.End = %v, past the last line %d", i, d.Loc.End, tt.lines)
				}
			}
			// The error really is anchored at the end of the input: that is
			// the case the phantom trailing line used to break.
			if got := diags[0].Loc.Span.Start; got != tt.wantEnd {
				t.Errorf("diags[0].Loc.Span.Start = %d, want %d (end of input)", got, tt.wantEnd)
			}
		})
	}
}

// TestDiagnosticWithoutLocation covers a diagnostic reported during
// realization, which works on content that has lost every link back to the
// syntax it came from and so has nothing to point at.
func TestDiagnosticWithoutLocation(t *testing.T) {
	_, _, err := writst.Compile([]byte("#show heading: it => int\n= T\n"))
	var diags writst.DiagnosticList
	if !errors.As(err, &diags) || len(diags) != 1 {
		t.Fatalf("Compile() = %v, want one error", err)
	}
	if diags[0].Loc.IsValid() {
		t.Errorf("Loc = %v, want no location", diags[0].Loc)
	}
	// A locationless diagnostic must still say something useful.
	if got := diags[0].Error(); got != diags[0].Msg {
		t.Errorf("Error() = %q, want %q", got, diags[0].Msg)
	}
}

// TestCompileWarningsCarryPositionsAndHints checks that warnings get the same
// treatment as errors, hints included.
func TestCompileWarningsCarryPositionsAndHints(t *testing.T) {
	_, warnings, err := writst.Compile([]byte("a\n#metadata(1) <l>\n#metadata(2) <l>"))
	if err != nil {
		t.Fatalf("Compile() = %v, want no error", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("got %d warnings, want 1: %v", len(warnings), warnings)
	}
	w := warnings[0]
	if w.Severity != writst.Warning {
		t.Errorf("Severity = %v, want %v", w.Severity, writst.Warning)
	}
	if want := (syntax.Position{Line: 3, Column: 2}); w.Loc.Start != want {
		t.Errorf("Loc.Start = %v, want %v", w.Loc.Start, want)
	}
	if len(w.Hints) != 1 {
		t.Errorf("Hints = %v, want one hint", w.Hints)
	}
}

func TestFormatDiagnostics(t *testing.T) {
	diags := []writst.Diagnostic{
		{
			Severity: writst.Error,
			Loc: syntax.Location{
				Span:  syntax.Span{Start: 20, End: 23},
				Start: syntax.Position{Line: 3, Column: 12},
				End:   syntax.Position{Line: 3, Column: 15},
			},
			Msg: "unknown variable: foo",
		},
		{
			Severity: writst.Error,
			Loc: syntax.Location{
				Span:  syntax.Span{Start: 40, End: 44},
				Start: syntax.Position{Line: 7, Column: 1},
				End:   syntax.Position{Line: 7, Column: 5},
			},
			Msg:   "missing argument: body",
			Hints: []string{"dates must be written as\ndatetime(year: 2024, month: 2, day: 29)"},
		},
		{
			Severity: writst.Warning,
			Loc: syntax.Location{
				Span:  syntax.Span{Start: 50, End: 51},
				Start: syntax.Position{Line: 9, Column: 3},
				End:   syntax.Position{Line: 9, Column: 4},
			},
			Msg: "content labelled multiple times",
		},
		{
			Severity: writst.Error,
			Msg:      "cannot convert integer to content",
		},
	}

	tests := []struct {
		name    string
		docName string
		want    string
	}{
		{
			name:    "with_name",
			docName: "doc.wr",
			want: `doc.wr:3:12: unknown variable: foo
doc.wr:7:1: missing argument: body
  hint: dates must be written as
        datetime(year: 2024, month: 2, day: 29)
doc.wr:9:3: warning: content labelled multiple times
doc.wr: cannot convert integer to content
`,
		},
		{
			name:    "without_name",
			docName: "",
			want: `3:12: unknown variable: foo
7:1: missing argument: body
  hint: dates must be written as
        datetime(year: 2024, month: 2, day: 29)
9:3: warning: content labelled multiple times
cannot convert integer to content
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sb strings.Builder
			if err := writst.FormatDiagnostics(&sb, tt.docName, diags); err != nil {
				t.Fatalf("FormatDiagnostics() = %v", err)
			}
			if got := sb.String(); got != tt.want {
				t.Errorf("FormatDiagnostics() =\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

// TestFormatDiagnosticsEndToEnd is the acceptance check for the whole change:
// compile a document, hold only Compile's return values, and render them.
// Nothing here touches the source bytes.
func TestFormatDiagnosticsEndToEnd(t *testing.T) {
	_, _, err := writst.Compile([]byte(threeErrors))
	var diags writst.DiagnosticList
	if !errors.As(err, &diags) {
		t.Fatalf("Compile() error is %T, want writst.DiagnosticList", err)
	}
	var sb strings.Builder
	if err := writst.FormatDiagnostics(&sb, "doc.wr", diags); err != nil {
		t.Fatalf("FormatDiagnostics() = %v", err)
	}
	want := `doc.wr:1:2: unknown variable: foo
doc.wr:3:2: unknown variable: bar
doc.wr:5:10: unknown variable: baz
`
	if got := sb.String(); got != want {
		t.Errorf("FormatDiagnostics() =\n%s\nwant:\n%s", got, want)
	}
}
