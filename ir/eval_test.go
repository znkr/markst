package ir_test

import (
	"flag"
	"path/filepath"
	"testing"
	"unique"

	"github.com/google/go-cmp/cmp"
	"znkr.io/diff/textdiff"
	"znkr.io/writst/internal/errcmp"
	"znkr.io/writst/internal/testfile"
	"znkr.io/writst/ir"
	"znkr.io/writst/ir/types"
	"znkr.io/writst/syntax"
	"znkr.io/writst/syntax/analyzer"
	"znkr.io/writst/syntax/parser"
)

var update = flag.Bool("update", false, "update golden files")

func TestEval(t *testing.T) {
	files, err := filepath.Glob("testdata/*.test")
	if err != nil {
		t.Fatal(err)
	}

	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			t.Parallel()
			tests := testfile.Read(t, file)
			for i, tc := range tests {
				t.Run(tc.Name, func(t *testing.T) {
					if tc.Skip != "" {
						t.Skip(tc.Skip)
					}

					root := parser.Parse(tc.Input)
					exprs, err := analyzer.Analyze(root)
					if err != nil {
						if diff := errcmp.Diff(root, analysisErrors(err)); diff != "" {
							t.Fatalf("Analyze() error mismatch (-want +got):\n%s", diff)
						}
						return
					}

					bindings := map[unique.Handle[string]]ir.Value{
						unique.Make("test"): &ir.Function{
							Name:       "test",
							Positional: []types.Set{types.Any, types.Any},
							F: func(fcc *ir.FuncCallContext, args []ir.Value, named ir.NamedArgsWithDefaults) (ir.Value, error) {
								got, want := args[0], args[1]
								if diff := cmp.Diff(ir.FormatValue(want), ir.FormatValue(got)); diff != "" {
									call := tc.Input[fcc.Span.Start:fcc.Span.End]
									t.Errorf("test failure. The following test failed:\n\n\t%s\n\nDiff (-want +got):\n%s", call, diff)
								}
								return ir.None{}, nil
							},
						},
					}

					contents, warnings, err := ir.Eval(exprs, ir.WithBindings(bindings))
					if diff := errcmp.Diff(root, evalErrors(warnings, err)); diff != "" {
						t.Errorf("Eval() error mismatch (-want +got):\n%s", diff)
					}
					if err != nil {
						return
					}

					got := ir.FormatContent(contents)
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

func evalErrors(warnings []ir.Error, err error) []errcmp.Error {
	var ret []errcmp.Error
	for _, w := range warnings {
		ret = append(ret, errcmp.Error{
			Span:    w.Span(),
			Type:    "Warning",
			Message: w.Error(),
			Hints:   w.Hints(),
		})
	}
	if err != nil {
		err0 := err.(ir.Error)
		ret = append(ret, errcmp.Error{
			Span:    err0.Span(),
			Type:    "Error",
			Message: err0.Error(),
			Hints:   err0.Hints(),
		})
	}
	return ret
}
