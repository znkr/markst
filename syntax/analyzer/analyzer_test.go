package analyzer_test

import (
	"flag"
	"path/filepath"
	"strings"
	"testing"

	"znkr.io/diff/textdiff"

	"znkr.io/markst/expr"
	"znkr.io/markst/internal/testfile"
	"znkr.io/markst/name"
	"znkr.io/markst/syntax/analyzer"
	"znkr.io/markst/syntax/parser"
	"znkr.io/markst/value"
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

// TestAnalyzeWithExports pins the shape WithExports produces: the top-level
// function returns [body, exports], and the dict names every top-level binding
// — closures included, which is the point: nothing must let dead-code
// elimination drop a function that only the export dict uses.
func TestAnalyzeWithExports(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			// Declared zeta-first to show the entries come back sorted:
			// bindings live in a map, and an arbitrary order would make the
			// same source analyze differently from run to run.
			name: "closures and values, name-sorted",
			src:  "#let zeta = 1\n#let alpha(x) = x\n",
			want: "v1 = make_dict (alpha: v0, zeta: 1)\n    v2 = make_array [none, v1]",
		},
		{
			name: "block-scoped bindings are not exported",
			src:  "#{ let hidden = 1 }\n#let shown = 2\n",
			want: "make_dict (shown: 2)",
		},
		{
			name: "a file that binds nothing exports nothing",
			src:  "hello\n",
			want: "make_dict ()",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := parser.Parse([]byte(tc.src))
			got := expr.FormatModule(analyzer.Analyze(root, analyzer.WithExports()))
			if !strings.Contains(got, tc.want) {
				t.Errorf("expected IR to contain:\n%s\ngot:\n%s", tc.want, got)
			}
		})
	}
}

// TestAnalyzeWithName checks that the name reaches the module, since every
// span the module ever produces is reported under it.
func TestAnalyzeWithName(t *testing.T) {
	root := parser.Parse([]byte("hello\n"))
	mod := analyzer.Analyze(root, analyzer.WithName("lib.mst"))
	if mod.Origin.Name != "lib.mst" {
		t.Errorf("Origin.Name = %q, want %q", mod.Origin.Name, "lib.mst")
	}
	if mod.Origin.Source == nil {
		t.Errorf("Origin.Source = nil; spans would not resolve")
	}
	// Without the option the source is still there, so spans stay locatable.
	if bare := analyzer.Analyze(parser.Parse([]byte("hello\n"))); bare.Origin.Source == nil {
		t.Errorf("Origin.Source = nil without WithName; spans would not resolve")
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
