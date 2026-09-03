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

import (
	"testing"

	"znkr.io/markst/internal/slab"
)

func TestArenaAcrossBlocks(t *testing.T) {
	// Enough nodes to cross several blocks.
	const n = 4 * slab.Block
	var a Arena
	leaves := make([]*Leaf, n)
	inners := make([]*Inner, n)
	for i := range n {
		span := Span{Start: uint32(i), End: uint32(i + 1)}
		leaves[i] = a.Leaf(KindText, span)
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
// to one must not write over the children of the node allocated next. Nodes are
// told apart by their spans, a node carrying nothing else of its own.
func TestArenaNodesAreSeparate(t *testing.T) {
	var a Arena
	leaf := func(i uint32) Node { return a.Leaf(KindText, Span{Start: i, End: i + 1}) }
	first := a.Nodes(2)
	second := a.Nodes(2)
	first[0], first[1] = leaf(0), leaf(1)
	second[0], second[1] = leaf(2), leaf(3)
	if got := append(first, leaf(4)); got[2].Span().Start != 4 {
		t.Fatalf("appending to a children slice wrote a node spanning %v", got[2].Span())
	}
	if second[0].Span().Start != 2 || second[1].Span().Start != 3 {
		t.Errorf("appending to one children slice overwrote another: %v, %v",
			second[0].Span(), second[1].Span())
	}
}

func TestArenaError(t *testing.T) {
	var a Arena
	e := a.Error(Span{Start: 1, End: 2}, "bad", "try harder")
	if e.Kind() != KindError || e.Message() != "bad" || e.Span() != (Span{Start: 1, End: 2}) {
		t.Errorf("Error() = %v %q %v", e.Kind(), e.Message(), e.Span())
	}
	e.Hint("and again")
	if got := e.Hints(); len(got) != 2 {
		t.Errorf("Hints() = %v, want two", got)
	}
}

func TestArenaConvert(t *testing.T) {
	var a Arena
	leaf := a.Leaf(KindText, Span{Start: 1, End: 2})
	if got := a.Convert(leaf, KindIdent); got.Kind() != KindIdent || got.Span() != leaf.Span() {
		t.Errorf("Convert(leaf) = %v %v", got.Kind(), got.Span())
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
	a.Convert(a.Error(Span{}, "bad"), KindText)
}

// TestArenaBlocks pins what a block buys: a run of nodes costs one allocation
// per block of them, not one each.
func TestArenaBlocks(t *testing.T) {
	const n = 4 * slab.Block
	allocs := testing.AllocsPerRun(10, func() {
		var a Arena
		for range n {
			a.Leaf(KindText, Span{})
		}
	})
	if want := float64(n / slab.Block); allocs != want {
		t.Errorf("%d leaves took %v allocations, want %v", n, allocs, want)
	}
}
