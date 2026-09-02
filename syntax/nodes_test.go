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
		{"empty", NewInner(KindMarkup, nil), Span{}},
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
