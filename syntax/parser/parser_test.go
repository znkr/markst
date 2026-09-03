// Copyright 2026 Florian Zenker (flo@znkr.io)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package parser_test

import (
	"flag"
	"path/filepath"
	"testing"

	"znkr.io/diff/textdiff"

	"znkr.io/markst/internal/testfile"
	"znkr.io/markst/syntax"
	"znkr.io/markst/syntax/internal/format"
	"znkr.io/markst/syntax/parser"
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

					node := parser.Parse([]byte(tc.Input))
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
					checkSpans(t, parser.Parse([]byte(tc.Input)))
				})
			}
		})
	}
}

// checkSpans asserts what every consumer of a tree relies on: a node's span is
// the source it covers. The root spans the whole input, so [syntax.Text] of it
// is the input back; every other node spans a well-formed range inside its
// parent's, in order, so [syntax.Text] of it is a slice of the source and never
// panics.
func checkSpans(t *testing.T, root syntax.RootNode) {
	t.Helper()
	if got := syntax.Text(root.Src, root); string(got) != string(root.Src) {
		t.Errorf("Text() != Input:\n  input: %q\n  got:   %q", string(root.Src), got)
	}
	var check func(n syntax.Node, parent syntax.Span)
	check = func(n syntax.Node, parent syntax.Span) {
		span := n.Span()
		if span.Start > span.End {
			t.Fatalf("%v: span runs backwards: %v", n.Kind(), span)
		}
		if span.Start < parent.Start || span.End > parent.End {
			t.Fatalf("%v: span %v escapes parent's %v", n.Kind(), span, parent)
		}
		inner, ok := n.(*syntax.Inner)
		if !ok {
			return
		}
		prev := span.Start
		for _, c := range inner.Children() {
			if c.Span().Start < prev {
				t.Fatalf("%v: child %v at %v overlaps the one before it", n.Kind(), c.Kind(), c.Span())
			}
			check(c, span)
			prev = c.Span().End
		}
	}
	check(root.Inner, syntax.Span{Start: 0, End: uint32(len(root.Src))})
}

// badSpans are inputs whose tree once had a node spanning the wrong source. They
// are seeded explicitly because the golden files cannot hold them: every input
// there ends in a newline, and it is a construct left empty at end of input that
// has no children to take a span from.
var badSpans = []string{
	"a *",      // Strong's empty Markup dragged its end to 0, inverting its span
	"a _",      // same, Emph
	"a #[",     // same, ContentBlock
	"a #{",     // same, CodeBlock's empty Code
	"a $#[",    // same, reached through math
	"= a\n\n*", // same, past a parbreak, so the span ran backwards by more than a line
}

// FuzzParse asserts that no input makes the parser panic or hang, and that the
// tree it produces spans the input the way checkSpans describes.
func FuzzParse(f *testing.F) {
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
	for _, src := range badSpans {
		f.Add(src)
	}

	f.Fuzz(func(t *testing.T, src string) {
		checkSpans(t, parser.Parse([]byte(src)))
	})
}
