package writst_test

import (
	"flag"
	"path/filepath"
	"testing"
	"unique"

	"znkr.io/diff/textdiff"
	"znkr.io/writst/expr"
	"znkr.io/writst/internal/errcmp"
	"znkr.io/writst/internal/testfile"
	"znkr.io/writst/syntax"
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
		name, err := filepath.Rel("testdata", file)
		if err != nil {
			t.Fatal(err)
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tests := testfile.Read(t, file)
			for i, tc := range tests {
				t.Run(tc.Name, func(t *testing.T) {
					if tc.Skip != "" {
						t.Skip(tc.Skip)
					}

					root := parser.Parse(tc.Input)
					exprs, err := analyzer.Analyze(root,
						analyzer.WithBindings(
							unique.Make("test"),
							unique.Make("dont-care"),
							unique.Make("nope"),
						),
					)
					if err != nil {
						if diff := errcmp.Diff(root, analysisErrors(err)); diff != "" {
							t.Fatalf("Analyze() error mismatch (-want +got):\n%s", diff)
						}
						return
					}

					bindings := map[unique.Handle[string]]value.Value{
						unique.Make("test"): &value.Function{
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
						unique.Make("dont-care"): nil,
					}

					contents, warnings, err := expr.Eval(exprs, expr.WithBindings(bindings))
					if diff := errcmp.Diff(root, evalErrors(warnings, err)); diff != "" {
						t.Errorf("Eval() error mismatch (-want +got):\n%s", diff)
					}
					if err != nil {
						return
					}

					got := value.FormatContent(contents)
					if diff := textdiff.Unified(tc.Want, got); diff != "" {
						t.Errorf("Analyze() mismatch (-want +got):\n%s", diff)
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

func analysisErrors(err error) []errcmp.Error {
	if err == nil {
		return nil
	}
	var ret []errcmp.Error
	for _, e := range err.(syntax.ErrorList) {
		ret = append(ret, errcmp.Error{
			Span:    e.Span(),
			Type:    "Error",
			Message: e.Error(),
			Hints:   e.Hints(),
		})
	}
	return ret

}

func evalErrors(warnings []expr.Error, err error) []errcmp.Error {
	var ret []errcmp.Error
	for _, w := range warnings {
		ret = append(ret, errcmp.Error{
			Span:    w.Span(),
			Type:    "Warning",
			Message: w.Error(),
			Hints:   w.Hints(),
		})
	}
	switch e := err.(type) {
	case nil:
	case expr.ErrorList:
		for _, e := range e {
			ret = append(ret, errcmp.Error{
				Span:    e.Span(),
				Type:    "Error",
				Message: e.Error(),
				Hints:   e.Hints(),
			})
		}
	case expr.Error:
		ret = append(ret, errcmp.Error{
			Span:    e.Span(),
			Type:    "Error",
			Message: e.Error(),
			Hints:   e.Hints(),
		})
	default:
		panic("unexpected error: " + e.Error())
	}
	return ret
}
