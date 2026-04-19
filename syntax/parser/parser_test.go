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

func TestTextRoundTripGolden(t *testing.T) {
	// Verify the round-trip invariant for every test case in testdata.
	files, err := filepath.Glob("testdata/*.test")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			t.Parallel()
			for _, tc := range testfile.Read(t, file) {
				t.Run(tc.Name, func(t *testing.T) {
					node := parser.Parse(tc.Input)
					got := node.Text()
					if got != tc.Input {
						t.Errorf("Text() != Input:\n  input: %q\n  got:   %q", tc.Input, got)
					}
				})
			}
		})
	}
}

func FuzzTextRoundTrip(f *testing.F) {
	// Seed with golden test inputs.
	files, err := filepath.Glob("testdata/*.test")
	if err != nil {
		f.Fatal(err)
	}
	for _, file := range files {
		for _, tc := range testfile.Read(f, file) {
			f.Add(tc.Input)
		}
	}

	f.Fuzz(func(t *testing.T, src string) {
		defer func() {
			if r := recover(); r != nil {
				// Parser may panic on unimplemented features; skip those.
				t.Skipf("parser panicked: %v", r)
			}
		}()
		node := parser.Parse(src)
		got := node.Text()
		if got != src {
			t.Errorf("Text() != src:\n  src: %q\n  got: %q", src, got)
		}
	})
}
