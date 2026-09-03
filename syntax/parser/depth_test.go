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
	"strings"
	"testing"

	"znkr.io/markst/syntax"
	"znkr.io/markst/syntax/parser"
)

const depthMsg = "maximum parsing depth exceeded"

// parseErrors returns every error node in the tree, in document order.
func parseErrors(n syntax.Node) []*syntax.Error {
	var errs []*syntax.Error
	var walk func(syntax.Node)
	walk = func(n syntax.Node) {
		switch n := n.(type) {
		case *syntax.Error:
			errs = append(errs, n)
		case *syntax.Inner:
			for _, c := range n.Children() {
				walk(c)
			}
		}
	}
	walk(n)
	return errs
}

func depthErrors(n syntax.Node) []*syntax.Error {
	var errs []*syntax.Error
	for _, e := range parseErrors(n) {
		if e.Message() == depthMsg {
			errs = append(errs, e)
		}
	}
	return errs
}

// nestings are sources that nest one construct inside itself n times. Each
// shape has to be caught by the depth limit, whichever of the parser's modes
// it recurses through.
var nestings = []struct {
	name string
	src  func(n int) string
}{
	{"code-parens", func(n int) string { return "#" + strings.Repeat("(", n) + "1" + strings.Repeat(")", n) }},
	{"code-blocks", func(n int) string { return "#" + strings.Repeat("{", n) + strings.Repeat("}", n) }},
	{"content-blocks", func(n int) string { return strings.Repeat("#[", n) + strings.Repeat("]", n) }},
	{"math-delimited", func(n int) string { return "$" + strings.Repeat("(", n) + strings.Repeat(")", n) + "$" }},
	{"destructuring", func(n int) string {
		return "#let " + strings.Repeat("(", n) + "x" + strings.Repeat(")", n) + " = 1"
	}},
	{"unclosed-parens", func(n int) string { return "#" + strings.Repeat("(", n) }},
	{"unclosed-content-blocks", func(n int) string { return strings.Repeat("#[", n) }},
}

func TestDepthWithinLimit(t *testing.T) {
	// 64 levels are within the limit for every shape, including those that
	// count more than one level of nesting per level of source.
	for _, tt := range nestings {
		t.Run(tt.name, func(t *testing.T) {
			src := tt.src(64)
			root := parser.Parse([]byte(src))
			if errs := depthErrors(root.Inner); len(errs) > 0 {
				t.Errorf("Parse(%s at 64 levels) reported %q", tt.name, errs[0].Message())
			}
		})
	}
}

func TestDepthExceeded(t *testing.T) {
	for _, tt := range nestings {
		t.Run(tt.name, func(t *testing.T) {
			src := tt.src(2048)
			root := parser.Parse([]byte(src))

			// One error for the whole construct the parser gave up on, not one
			// per token of it.
			if errs := depthErrors(root.Inner); len(errs) != 1 {
				t.Errorf("Parse(%s at 2048 levels) reported %d depth errors, want 1", tt.name, len(errs))
			}
			if got := syntax.Text(root.Src, root.Inner); string(got) != src {
				t.Errorf("Parse(%s at 2048 levels) does not round-trip:\n  len(src) = %d\n  len(got) = %d", tt.name, len(src), len(got))
			}
		})
	}
}

func TestDepthLimitBoundary(t *testing.T) {
	// Each paren costs one level, on top of the one the surrounding markup
	// takes, so the deepest source that still parses stops two short of the
	// limit.
	src := func(n int) string { return "#" + strings.Repeat("(", n) + "1" + strings.Repeat(")", n) }
	for _, tt := range []struct {
		levels int
		want   int
	}{
		{253, 0},
		{254, 0},
		{255, 1},
		{256, 1},
	} {
		root := parser.Parse([]byte(src(tt.levels)))
		if got := len(depthErrors(root.Inner)); got != tt.want {
			t.Errorf("Parse(%d nested parens) reported %d depth errors, want %d", tt.levels, got, tt.want)
		}
	}
}

// TestDepthLimitStopsStackOverflow parses input deep enough to exhaust the
// goroutine stack without a limit, which crashes the process rather than
// panicking.
func TestDepthLimitStopsStackOverflow(t *testing.T) {
	for _, tt := range nestings {
		t.Run(tt.name, func(t *testing.T) {
			parser.Parse([]byte(tt.src(1 << 20)))
		})
	}
}
