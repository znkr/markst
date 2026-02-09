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
	"znkr.io/writst/model/ir"
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

					node := parser.Parse(tc.Input)
					content, err := analyzer.Analyze(node)
					if diff := errcmp.Diff(node, err); diff != "" {
						t.Errorf("Analyze() error mismatch (-want +got):\n%s", diff)
					}

					ec := ir.NewEvalContext()
					ec.Bind(unique.Make("test"), &ir.Function{
						Name:          "test",
						NumPositional: 2,
						F: func(args *ir.Arguments) ir.Value {
							got, want := args.Positional[0], args.Positional[1]
							if diff := cmp.Diff(ir.FormatValue(got), ir.FormatValue(want)); diff != "" {
								t.Errorf("test() failed (-got +want):\n%s", diff)
							}
							return ir.None{}
						},
					})
					ec.PushScope()

					value := content.Eval(ec)
					got := ir.FormatValue(value)

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
