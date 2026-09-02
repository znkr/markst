package syntax

import "testing"

func TestArenaAcrossBlocks(t *testing.T) {
	// Enough nodes to cross several blocks.
	const n = 4 * blockSize
	var a Arena
	leaves := make([]*Leaf, n)
	inners := make([]*Inner, n)
	for i := range n {
		span := Span{Start: uint32(i), End: uint32(i + 1)}
		leaves[i] = a.Leaf(KindText, span, "x")
		kids := a.Nodes(1)
		kids[0] = leaves[i]
		inners[i] = a.Inner(KindMarkup, kids)
	}
	// Every node handed out earlier is still itself: a new block must not
	// disturb the one before it.
	for i := range n {
		if got := leaves[i].Span().Start; got != uint32(i) {
			t.Fatalf("leaf %d has span %d, want %d", i, got, i)
		}
		if got := inners[i].Children()[0]; got != Node(leaves[i]) {
			t.Fatalf("inner %d holds %v, want its own leaf", i, got)
		}
	}
}

// TestArenaNodesAreSeparate checks the capacity of a children slice: appending
// to one must not write over the children of the node allocated next.
func TestArenaNodesAreSeparate(t *testing.T) {
	var a Arena
	first := a.Nodes(2)
	second := a.Nodes(2)
	first[0], first[1] = a.Leaf(KindText, Span{}, "a"), a.Leaf(KindText, Span{}, "b")
	second[0], second[1] = a.Leaf(KindText, Span{}, "c"), a.Leaf(KindText, Span{}, "d")
	if got := append(first, a.Leaf(KindText, Span{}, "e")); got[2].Text() != "e" {
		t.Fatalf("appending to a children slice wrote %q", got[2].Text())
	}
	if second[0].Text() != "c" || second[1].Text() != "d" {
		t.Errorf("appending to one children slice overwrote another: %q, %q",
			second[0].Text(), second[1].Text())
	}
}

func TestArenaError(t *testing.T) {
	var a Arena
	e := a.Error(Span{Start: 1, End: 2}, "bad", "x", "try harder")
	if e.Kind() != KindError || e.Message() != "bad" || e.Text() != "x" {
		t.Errorf("Error() = %v %q %q", e.Kind(), e.Message(), e.Text())
	}
	e.Hint("and again")
	if got := e.Hints(); len(got) != 2 {
		t.Errorf("Hints() = %v, want two", got)
	}
}

func TestArenaConvert(t *testing.T) {
	var a Arena
	leaf := a.Leaf(KindText, Span{Start: 1, End: 2}, "x")
	if got := a.Convert(leaf, KindIdent); got.Kind() != KindIdent || got.Span() != leaf.Span() || got.Text() != "x" {
		t.Errorf("Convert(leaf) = %v %v %q", got.Kind(), got.Span(), got.Text())
	}
	kids := a.Nodes(1)
	kids[0] = leaf
	inner := a.Inner(KindMarkup, kids)
	if got := a.Convert(inner, KindCode); got.Kind() != KindCode || got.Span() != inner.Span() {
		t.Errorf("Convert(inner) = %v %v", got.Kind(), got.Span())
	}
	defer func() {
		if recover() == nil {
			t.Error("Convert() of an error node did not panic")
		}
	}()
	a.Convert(a.Error(Span{}, "bad", ""), KindText)
}

// TestArenaBlocks pins what a block buys: a run of nodes costs one allocation
// per block of them, not one each.
func TestArenaBlocks(t *testing.T) {
	const n = 4 * blockSize
	allocs := testing.AllocsPerRun(10, func() {
		var a Arena
		for range n {
			a.Leaf(KindText, Span{}, "x")
		}
	})
	if want := float64(n / blockSize); allocs != want {
		t.Errorf("%d leaves took %v allocations, want %v", n, allocs, want)
	}
}
