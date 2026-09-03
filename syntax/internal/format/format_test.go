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

package format

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"znkr.io/markst/syntax"
)

func TestFormat(t *testing.T) {
	// A node's text is the source it spans, so the tree is built over a source
	// its spans line up with.
	src := []byte("hello*world*literal")
	node := syntax.NewInner(syntax.KindMarkup, []syntax.Node{
		syntax.NewLeaf(syntax.KindText, syntax.Span{Start: 0, End: 5}),
		syntax.NewInner(syntax.KindStrong, []syntax.Node{
			syntax.NewLeaf(syntax.KindStar, syntax.Span{Start: 5, End: 6}),
			syntax.NewLeaf(syntax.KindText, syntax.Span{Start: 6, End: 11}),
			syntax.NewLeaf(syntax.KindStar, syntax.Span{Start: 11, End: 12}),
		}),
		syntax.NewError(
			syntax.Span{Start: 12, End: 19},
			"something went wrong",
			"try again",
		),
	})

	expected := `Markup
  Text: "hello"
  Strong
    Star: "*"
    Text: "world"
    Star: "*"
  Error: "literal"
    Msg: "something went wrong"
    Hint: "try again"
`

	got := Format(syntax.RootNode{Src: src, Inner: node})
	if diff := cmp.Diff(expected, got); diff != "" {
		t.Errorf("Format() mismatch (-want +got):\n%s", diff)
	}
}
