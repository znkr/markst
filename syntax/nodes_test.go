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

package syntax

import "testing"

func TestInnerSpan(t *testing.T) {
	leaf := func(start, end uint32) Node {
		return NewLeaf(KindText, Span{Start: start, End: end})
	}

	tests := []struct {
		name string
		node *Inner
		want Span
	}{
		{"empty", NewEmptyInner(KindMarkup, 7), Span{Start: 7, End: 7}},
		{"single child", NewInner(KindMarkup, []Node{leaf(2, 5)}), Span{Start: 2, End: 5}},
		{
			"spans first through last child",
			NewInner(KindMarkup, []Node{leaf(2, 5), leaf(5, 6), leaf(6, 9)}),
			Span{Start: 2, End: 9},
		},
		{
			"nested",
			NewInner(KindMarkup, []Node{
				NewInner(KindStrong, []Node{leaf(0, 1), leaf(1, 2)}),
				NewInner(KindEmph, []Node{leaf(2, 3), leaf(3, 4)}),
			}),
			Span{Start: 0, End: 4},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.node.Span(); got != tt.want {
				t.Errorf("Span() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestInnerSpanDeeplyNested guards the cost of [Inner.Span]. It used to recurse
// into both the first and the last child on every call, which costs 2^depth for
// a deeply nested tree — enough to make analyzing `#{{{…` with thirty braces
// take half a minute. If that regresses, this test does not fail, it hangs.
func TestInnerSpanDeeplyNested(t *testing.T) {
	const depth = 64
	var n Node = NewLeaf(KindText, Span{Start: 0, End: depth})
	for range depth {
		n = NewInner(KindMarkup, []Node{n})
	}
	if got, want := n.Span(), (Span{Start: 0, End: depth}); got != want {
		t.Errorf("Span() = %v, want %v", got, want)
	}
}
