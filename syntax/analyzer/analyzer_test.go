package analyzer_test

import (
	"flag"
	"path/filepath"
	"testing"

	"znkr.io/diff/textdiff"

	"znkr.io/writst/internal/errcmp"
	"znkr.io/writst/internal/testfile"
	"znkr.io/writst/model/ir"
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
			tests := testfile.Read(t, file)
			for i, tc := range tests {
				t.Run(tc.Name, func(t *testing.T) {
					if tc.Skip != "" {
						t.Skip(tc.Skip)
					}

					node := parser.Parse(tc.Input)
					content, err := analyzer.Analyze(node)
					if err != nil {
						if diff := errcmp.Diff(node, err); diff != "" {
							t.Errorf("Analyze() error mismatch (-want +got):\n%s", diff)
						}
						return
					}

					got := ir.Format(content)
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
