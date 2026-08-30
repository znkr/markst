package writst_test

import (
	"flag"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"znkr.io/diff/textdiff"
	"znkr.io/writst/eval"
	"znkr.io/writst/expr"
	"znkr.io/writst/internal/errcmp"
	"znkr.io/writst/internal/testfile"
	"znkr.io/writst/name"
	"znkr.io/writst/syntax"
	"znkr.io/writst/syntax/analyzer"
	"znkr.io/writst/syntax/parser"
	"znkr.io/writst/types"
	"znkr.io/writst/value"
)

var update = flag.Bool("update", false, "update golden files")

// testNow is the instant the corpus is evaluated at. `datetime.today()` reads
// the current date off it, so tests that print today's date have something
// stable to expect. Noon on the epoch is deliberate: it leaves room on both
// sides of the day boundary for the offsets the datetime tests apply.
var testNow = time.Date(1970, 1, 1, 12, 0, 0, 0, time.UTC)

func TestWritst(t *testing.T) {
	files, err := filepath.Glob("testdata/**/*.test")
	if err != nil {
		t.Fatal(err)
	}

	for _, file := range files {
		testname, err := filepath.Rel("testdata", file)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(testname, func(t *testing.T) {
			t.Parallel()
			tests := testfile.Read(t, file)
			for i, tc := range tests {
				t.Run(tc.Name, func(t *testing.T) {
					if tc.Skip != "" {
						t.Skip(tc.Skip)
					}

					var mod *expr.Module

					bindings := map[name.Name]value.Value{
						name.Make("test"): &value.Function{
							Name:       "test",
							Positional: []value.Param{{Type: types.Any}, {Type: types.Any}},
							F: func(fcc *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
								got, want := args[0], args[1]
								if !value.Equal(got, want) {
									t.Errorf("%s", formatTestFailure(tc.Input, fcc.Span, mod, want, got))
								}
								return value.None{}, nil
							},
						},
						name.Make("dont-care"): &value.Error{
							Msg: "evaluated placeholder dont-care: this is an error",
						},
						name.Make("nope"): &value.Error{
							Msg: "evaluated placeholder nope: this is an error",
						},
					}

					root := parser.Parse([]byte(tc.Input))
					mod = analyzer.Analyze(root, analyzer.WithBindings(bindings))
					contents, warnings, errors := eval.Eval(mod, eval.WithNow(testNow))

					if diff := errcmp.Diff(root, evalErrors(warnings, errors)); diff != "" {
						t.Errorf("error mismatch (-want +got):\n%s", diff)
					}
					if errors != nil {
						return
					}

					got := value.FormatContent(contents)
					if diff := textdiff.Unified(tc.Want, got); diff != "" {
						t.Errorf("Eval() mismatch (-want +got):\n%s", diff)
					}

					if *update {
						tests[i].Want = got
					}
				})
			}
			if *update {
				testfile.Update(t, file, tests)
			}
		})
	}
}

// TestApproximationsPreserveOutput runs the whole corpus through the analyzer
// twice — once normally, once with [analyzer.WithoutApproximations] — and
// requires the two to evaluate to the same document and the same diagnostics.
//
// The approximations exist to lower less; the module they produce is smaller
// and its value numbering differs, which is exactly what the analyzer's own
// golden files record. What must not differ is anything a reader of the
// document can observe, and that is what this asserts. It runs by default, so
// an approximation that is not output-preserving fails the ordinary test run.
func TestApproximationsPreserveOutput(t *testing.T) {
	files, err := filepath.Glob("testdata/**/*.test")
	if err != nil {
		t.Fatal(err)
	}

	// The corpus binds `test` to an assertion helper and two names to error
	// placeholders. Here the assertions are somebody else's job — TestWritst
	// makes them — but the names still have to resolve the same way, or the two
	// runs would differ for a reason that has nothing to do with lowering.
	bindings := map[name.Name]value.Value{
		name.Make("test"): &value.Function{
			Name:       "test",
			Positional: []value.Param{{Type: types.Any}, {Type: types.Any}},
			F: func(*value.FunctionCallContext, []value.Value, value.NamedArgsWithDefaults) (value.Value, error) {
				return value.None{}, nil
			},
		},
		name.Make("dont-care"): &value.Error{
			Msg: "evaluated placeholder dont-care: this is an error",
		},
		name.Make("nope"): &value.Error{
			Msg: "evaluated placeholder nope: this is an error",
		},
	}

	// render evaluates src and returns everything observable about the result:
	// the formatted document, and the diagnostics in the order they were
	// reported.
	render := func(src string, opts ...analyzer.Option) string {
		root := parser.Parse([]byte(src))
		mod := analyzer.Analyze(root, append([]analyzer.Option{analyzer.WithBindings(bindings)}, opts...)...)
		contents, warnings, errors := eval.Eval(mod, eval.WithNow(testNow))
		var sb strings.Builder
		if contents != nil {
			sb.WriteString(value.FormatContent(contents))
		}
		for _, d := range evalErrors(warnings, errors) {
			fmt.Fprintf(&sb, "\n%v", d)
		}
		return sb.String()
	}

	for _, file := range files {
		testname, err := filepath.Rel("testdata", file)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(testname, func(t *testing.T) {
			t.Parallel()
			for _, tc := range testfile.Read(t, file) {
				t.Run(tc.Name, func(t *testing.T) {
					if tc.Skip != "" {
						t.Skip(tc.Skip)
					}
					with := render(tc.Input)
					without := render(tc.Input, analyzer.WithoutApproximations())
					if diff := textdiff.Unified(with, without); diff != "" {
						t.Errorf("lowering approximations changed the result (-with +without):\n%s", diff)
					}
				})
			}
		})
	}
}

// formatTestFailure renders a debugging report for a failed test(got, want)
// call: the failing call, the full test input with the failing row highlighted,
// the want/got values, and the SSA dump of the evaluated module.
func formatTestFailure(input string, span syntax.Span, mod *expr.Module, want, got value.Value) string {
	var b strings.Builder
	fmt.Fprintf(&b, "test failure: %s\n\n", input[span.Start:span.End])
	b.WriteString(highlightSource(input, span))
	fmt.Fprintf(&b, "\nwant: %s\n got: %s\n",
		strings.TrimRight(value.FormatValue(want), "\n"),
		strings.TrimRight(value.FormatValue(got), "\n"))
	fmt.Fprintf(&b, "\nIR:\n%s", expr.FormatModule(mod))
	return b.String()
}

// highlightSource renders input with 1-based line numbers, marking every row
// overlapped by span with a leading '>'.
func highlightSource(input string, span syntax.Span) string {
	var b strings.Builder
	offset := uint32(0)
	for i, line := range strings.SplitAfter(input, "\n") {
		if line == "" {
			break
		}
		end := offset + uint32(len(line))
		marker := ' '
		if span.Start < end && span.End > offset {
			marker = '>'
		}
		fmt.Fprintf(&b, "%c %3d │ %s\n", marker, i+1, strings.TrimRight(line, "\n"))
		offset = end
	}
	return b.String()
}

func evalErrors(warnings []eval.Error, errors []eval.Error) []errcmp.Error {
	var ret []errcmp.Error
	for _, w := range warnings {
		ret = append(ret, errcmp.Error{
			Span:    w.Span,
			Type:    "Warning",
			Message: w.Error(),
			Hints:   w.Hints,
		})
	}
	for _, e := range errors {
		ret = append(ret, errcmp.Error{
			Span:    e.Span,
			Type:    "Error",
			Message: e.Error(),
			Hints:   e.Hints,
		})
	}
	return ret
}
