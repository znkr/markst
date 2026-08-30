package analyzer_test

import (
	"flag"
	"path/filepath"
	"strings"
	"testing"

	"znkr.io/diff/textdiff"

	"znkr.io/writst/expr"
	"znkr.io/writst/internal/testfile"
	"znkr.io/writst/name"
	"znkr.io/writst/syntax/analyzer"
	"znkr.io/writst/syntax/parser"
	"znkr.io/writst/value"
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

					// The .test format terminates every input with a newline.
					// That terminator scans as a markup space and lowers to a
					// `const text " "`, which every case would then carry — noise
					// that also renumbers every value after it. Realization trims
					// it anyway (see testdata/text/spacing.test), so drop it here
					// and keep the golden about the lowering under test.
					root := parser.Parse([]byte(strings.TrimSuffix(tc.Input, "\n")))
					mod := analyzer.Analyze(root)
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

func TestAnalyzeWithBindings(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		bindings map[name.Name]value.Value
		// wantSubstr is a substring that must appear in the formatted IR; it
		// is the simplest assertion that proves a bound name resolved to its
		// supplied value rather than the default "unknown variable" error.
		wantSubstr string
		// wantNoSubstr asserts that no analyzer error was emitted for the
		// bound name (regression guard against future "declare-only" leaks).
		wantNoSubstr string
	}{
		{
			name:         "supplied value resolves as constant",
			src:          "#{ injected }",
			bindings:     map[name.Name]value.Value{name.Make("injected"): value.Str("hello")},
			wantSubstr:   `return hello`,
			wantNoSubstr: "unknown variable",
		},
		{
			name:       "unbound name still errors",
			src:        "#{ notbound }",
			bindings:   map[name.Name]value.Value{name.Make("other"): value.Str("x")},
			wantSubstr: "unknown variable: notbound",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := parser.Parse([]byte(tc.src))
			mod := analyzer.Analyze(root, analyzer.WithBindings(tc.bindings))
			got := expr.FormatModule(mod)
			if tc.wantSubstr != "" && !strings.Contains(got, tc.wantSubstr) {
				t.Errorf("expected IR to contain %q, got:\n%s", tc.wantSubstr, got)
			}
			if tc.wantNoSubstr != "" && strings.Contains(got, tc.wantNoSubstr) {
				t.Errorf("expected IR not to contain %q, got:\n%s", tc.wantNoSubstr, got)
			}
		})
	}
}
