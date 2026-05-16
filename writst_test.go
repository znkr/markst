package writst_test

import (
	"flag"
	"path/filepath"
	"testing"

	"znkr.io/diff/textdiff"
	"znkr.io/writst/eval"
	"znkr.io/writst/internal/errcmp"
	"znkr.io/writst/internal/testfile"
	"znkr.io/writst/name"
	"znkr.io/writst/syntax/analyzer"
	"znkr.io/writst/syntax/parser"
	"znkr.io/writst/types"
	"znkr.io/writst/value"
)

var update = flag.Bool("update", false, "update golden files")

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

					bindings := map[name.Name]value.Value{
						name.Make("test"): &value.Function{
							Name:       "test",
							Positional: []value.Param{{Type: types.Any}, {Type: types.Any}},
							F: func(fcc *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
								got, want := args[0], args[1]
								if !value.Equal(got, want) {
									call := tc.Input[fcc.Span.Start:fcc.Span.End]
									t.Errorf("test failure: %s\n\n\twant: %s\t got: %s", call, value.FormatValue(want), value.FormatValue(got))
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

					root := parser.Parse(tc.Input)
					mod := analyzer.Analyze(root, analyzer.WithBindings(bindings))
					contents, warnings, errors := eval.Eval(mod)

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
