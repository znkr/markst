package analyzer_test

import (
	"flag"
	"path/filepath"
	"testing"

	"znkr.io/diff/textdiff"

	"znkr.io/writst/expr"
	"znkr.io/writst/internal/errcmp"
	"znkr.io/writst/internal/testfile"
	"znkr.io/writst/syntax"
	"znkr.io/writst/syntax/analyzer"
	"znkr.io/writst/syntax/parser"
)

var update = flag.Bool("update", false, "update golden files")

func TestAnalyze(t *testing.T) {
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
					mod, err := analyzer.Analyze(node)
					if diff := errcmp.Diff(node, toCmpErrors(err)); diff != "" {
						t.Fatalf("Analyze() error mismatch (-want +got):\n%s", diff)
					}
					if err != nil {
						return
					}

					got := expr.FormatModule(mod)
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

func toCmpErrors(err error) []errcmp.Error {
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
