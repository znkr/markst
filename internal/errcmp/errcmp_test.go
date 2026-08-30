package errcmp

import (
	"strings"
	"testing"

	"znkr.io/writst/syntax"
	"znkr.io/writst/syntax/parser"
)

// wantError defines an expected error in human-readable form.
type wantError struct {
	Line    int    // 1-based line number, defaults to 1
	Start   int    // 1-based start column
	End     int    // 1-based end column, defaults to Start (single column)
	Type    string // "Error" or "Warning", defaults to "Error"
	Message string
	Hints   []string
}

func (e wantError) toError(root syntax.RootNode) Error {
	line, end, typ := e.Line, e.End, e.Type
	if line == 0 {
		line = 1
	}
	if end == 0 {
		end = e.Start
	}
	if typ == "" {
		typ = "Error"
	}
	return Error{
		Span: syntax.Span{
			Start: root.Source.Offset(syntax.Position{Line: uint32(line), Column: uint32(e.Start)}),
			End:   root.Source.Offset(syntax.Position{Line: uint32(line), Column: uint32(end)}),
		},
		Type:    typ,
		Message: e.Message,
		Hints:   e.Hints,
	}
}

func TestDiff(t *testing.T) {
	tests := []struct {
		name         string
		src          string
		got          []wantError
		wantEmpty    bool
		wantContains []string
	}{
		{
			name:      "matching_errors",
			src:       "// Error: 2-8 invalid hexadecimal number: 0x123z\n#0x123z",
			got:       []wantError{{Line: 2, Start: 2, End: 8, Message: "invalid hexadecimal number: 0x123z"}},
			wantEmpty: true,
		},
		{
			name:      "no_errors_expected_none_got",
			src:       "#let x = 1",
			wantEmpty: true,
		},
		{
			name: "unexpected_error",
			src:  "#let x = 1",
			got:  []wantError{{Start: 1, End: 5, Message: "something wrong"}},
			wantContains: []string{
				"+// Error: 1-5 something wrong",
				" #let x = 1",
			},
		},
		{
			name: "missing_expected_error",
			src:  "// Error: 2-4 bad value\n#1u",
			wantContains: []string{
				"-// Error: 2-4 bad value",
				" #1u",
			},
		},
		{
			name: "wrong_message",
			src:  "// Error: 2-4 bad value\n#1u",
			got:  []wantError{{Line: 2, Start: 2, End: 4, Message: "wrong message"}},
			wantContains: []string{
				"-// Error: 2-4 bad value",
				"+// Error: 2-4 wrong message",
				" #1u",
			},
		},
		{
			name: "wrong_span",
			src:  "// Error: 2-4 bad value\n#1u",
			got:  []wantError{{Line: 2, Start: 1, End: 3, Message: "bad value"}},
			wantContains: []string{
				"-// Error: 2-4 bad value",
				"+// Error: 1-3 bad value",
				" #1u",
			},
		},
		{
			name: "error_on_different_line",
			src:  "// Error: 1-4 bad value\n#let x = 1\n#let y = 2",
			got:  []wantError{{Line: 3, Start: 1, End: 4, Message: "bad value"}},
			wantContains: []string{
				"+#let x = 1",
				" // Error: 1-4 bad value",
				"-#let x = 1",
				" #let y = 2",
			},
		},
		{
			name: "warning_type",
			src:  "#let x = 1",
			got:  []wantError{{Start: 1, End: 5, Type: "Warning", Message: "unused variable"}},
			wantContains: []string{
				"+// Warning: 1-5 unused variable",
				" #let x = 1",
			},
		},
		{
			name: "single_column_span",
			src:  "#let x = 1",
			got:  []wantError{{Start: 5, Message: "unexpected"}},
			wantContains: []string{
				"+// Error: 5 unexpected",
				" #let x = 1",
			},
		},
		{
			name: "error_with_hints",
			src:  "#let x = 1",
			got:  []wantError{{Start: 1, End: 5, Message: "type mismatch", Hints: []string{"expected int", "got string"}}},
			wantContains: []string{
				"+// Error: 1-5 type mismatch",
				"+// Hint: 1-5 expected int",
				"+// Hint: 1-5 got string",
				" #let x = 1",
			},
		},
		{
			name: "multiple_errors_on_same_line",
			src:  "#let x = 1 + true",
			got: []wantError{
				{Start: 1, End: 5, Message: "first error"},
				{Start: 10, End: 14, Message: "second error"},
			},
			wantContains: []string{
				"+// Error: 1-5 first error",
				"+// Error: 10-14 second error",
				" #let x = 1 + true",
			},
		},
		{
			name: "error_past_eof",
			src:  "#for v\n",
			got:  []wantError{{Line: 2, Start: 1, Message: "expected something"}},
			wantContains: []string{
				" #for v",
				"+// Error: 1 expected something",
			},
		},
		{
			name: "error_on_stripped_comment_line",
			src:  "#let x = 1\n// Error: 5 wrong expectation\n#let y = 2",
			got:  []wantError{{Line: 2, Start: 1, End: 5, Message: "actual error on comment line"}},
			wantContains: []string{
				"-// Error: 5 wrong expectation",
				"+// Error: 1-5 actual error on comment line",
			},
		},
		{
			name: "error_on_only_stripped_line",
			src:  "// Error: 5 expected pattern\n#for v",
			got:  []wantError{{Line: 2, Start: 1, Message: "actual error here"}},
			wantContains: []string{
				"-// Error: 5 expected pattern",
				"+// Error: 1 actual error here",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := parser.Parse([]byte(tc.src))

			var got []Error
			for _, e := range tc.got {
				got = append(got, e.toError(root))
			}

			diff := Diff(root, got)

			if tc.wantEmpty {
				if diff != "" {
					t.Fatalf("Diff() returned non-empty:\n%s", diff)
				}
				return
			}

			if diff == "" {
				t.Fatal("Diff() returned empty, want non-empty diff")
			}
			for _, s := range tc.wantContains {
				if !containsLine(diff, s) {
					t.Errorf("Diff() missing line %q in:\n%s", s, diff)
				}
			}
		})
	}
}

func containsLine(s, line string) bool {
	for l := range strings.SplitSeq(s, "\n") {
		if l == line {
			return true
		}
	}
	return false
}
