package parser_test

import (
	"flag"
	"path/filepath"
	"testing"

	"znkr.io/diff/textdiff"

	"znkr.io/writst/internal/testfile"
	"znkr.io/writst/syntax/internal/format"
	"znkr.io/writst/syntax/parser"
)

var update = flag.Bool("update", false, "update golden files")

func TestParse(t *testing.T) {
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
					got := format.Format(node)

					if diff := textdiff.Unified(tc.Want, got); diff != "" {
						t.Errorf("Parse() mismatch (-want +got):\n%s", diff)
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

func TestTextRoundTrip(t *testing.T) {
	// Verify that root.Text() returns exactly the original source.
	// This invariant is critical for error position reporting to work correctly.
	tests := []struct {
		name string
		src  string
	}{
		{"empty", ""},
		{"simple_text", "hello world"},
		{"simple_code", "#let x = 1"},
		{"multiline", "line1\nline2\nline3"},
		{"for_loop_valid", "#for v in iter { v }"},
		{"for_loop_missing_pattern", "#for"},
		{"for_loop_incomplete", "#for v"},
		{"for_loop_missing_in", "#for v iter"},
		{"for_loop_missing_iterable", "#for v in"},
		{"for_loop_missing_body", "#for v in iter"},
		{"multiple_for_errors", "#for\n\n#for"},
		{"for_with_comments", "// Error: 5 expected pattern\n#for\n\n// Error: 5 expected pattern\n#for"},
		{"nested_errors", "#for #for"},
		{"error_with_text", "Hello #0xG world"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			node := parser.Parse(tc.src)
			got := node.Text()
			if got != tc.src {
				t.Errorf("Text() mismatch:\n  src: %q\n  got: %q", tc.src, got)
			}
		})
	}
}
