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
						if diff := errcmp.Diff(root, err); diff != "" {
							t.Errorf("Analyze() error mismatch (-want +got):\n%s", diff)
						}
						return
					}

					ec := ir.NewEvalContext()
					ec.Bind(unique.Make("test"), &ir.Function{
						Name:          "test",
						NumPositional: 2,
						F: func(args []ir.Value, named ir.NamedArgsWithDefaults) (ir.Value, error) {
							got, want := args[0], args[1]
							if diff := cmp.Diff(ir.FormatValue(want), ir.FormatValue(got)); diff != "" {
								t.Errorf("test() failed (-want +got):\n%s", diff)
							}
							return ir.None{}, nil
						},
					})

					contents, err := ir.Eval(ec, exprs)
					if diff := errcmp.Diff(root, err); diff != "" {
						t.Errorf("Eval() error mismatch (-want +got):\n%s", diff)
					}
					if err != nil {
						return
					}

					got := ir.FormatContents(contents)
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
