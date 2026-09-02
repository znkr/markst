package value

import "iter"

// Tree is a content tree to walk: a [Document], or an [Index] built over one.
// A function takes a Tree rather than a [Content] when the caller may already
// have an index to spend — the two walk the same elements in the same order, so
// which one is passed changes only what the walk costs.
type Tree interface {
	// Preorder walks the tree; see [Preorder] and [Index.Preorder].
	Preorder(kinds KindSet) iter.Seq[Cursor]
}

// Index is a content tree recorded flat, in document order: one entry per
// element, holding the element, how deep it sits, where its subtree ends, and
// the union of the kinds that subtree holds.
//
// It walks like the tree itself — [Index.Preorder] yields the same cursors in
// the same order as [Preorder] — and adds the one thing a walk cannot do: a
// subtree whose kinds miss the mask is stepped over whole, rather than
// descended into and filtered out. That is worth having when a caller walks the
// same document more than once, since the index is built once and every walk
// after that reads it.
//
// An index records the tree as it was when it was built. Nothing detects a tree
// that has changed since: an index of a modified tree walks elements that are
// no longer in it and misses the ones that are. Fields that carry no structure
// — a label, a heading's depth — may be written through a cursor as usual; it
// is adding, removing or replacing content that invalidates the index. Build a
// new one after that.
//
// The zero Index is empty and walks nothing. Build one with [NewIndex], or fill
// one in place with [Index.Init].
type Index struct {
	nodes []indexNode
}

// indexNode is one element's record. It is 40 bytes, so an index of a document
// costs about as much as the document's own spine.
type indexNode struct {
	c       Content
	end     int32   // one past the last element of this element's subtree
	depth   int32   // how many elements this one sits under
	subtree KindSet // the kinds in this element's subtree, its own included
	kind    ElemKind
}

// NewIndex returns an index of c and everything below it, in the order
// [Preorder] walks them.
func NewIndex(c Content) Index {
	var x Index
	x.Init(c)
	return x
}

// Init fills x with an index of c, reusing whatever x already allocated. It is
// how a caller that holds an index across documents avoids building a new one
// each time.
func (x *Index) Init(c Content) {
	x.nodes = x.nodes[:0]
	// open holds the elements whose subtrees are still being read, outermost
	// first: each is closed — its end and its subtree's kinds settled — once
	// the walk comes back out to a depth it no longer covers.
	var open []int32
	closeTop := func() {
		i := open[len(open)-1]
		open = open[:len(open)-1]
		x.nodes[i].end = int32(len(x.nodes))
		if len(open) > 0 {
			parent := open[len(open)-1]
			x.nodes[parent].subtree |= x.nodes[i].subtree
		}
	}
	for cur := range Preorder(c, AnyKind) {
		for len(open) > cur.Depth() {
			closeTop()
		}
		open = append(open, int32(len(x.nodes)))
		x.nodes = append(x.nodes, indexNode{
			c:       cur.Node(),
			depth:   int32(cur.Depth()),
			subtree: SetOf(cur.Kind()),
			kind:    cur.Kind(),
		})
	}
	for len(open) > 0 {
		closeTop()
	}
}

// Len returns the number of elements the index holds.
func (x *Index) Len() int { return len(x.nodes) }

// Root returns the element the index was built over, and nil for an empty index.
func (x *Index) Root() Content {
	if len(x.nodes) == 0 {
		return nil
	}
	return x.nodes[0].c
}

// Preorder walks the indexed tree, yielding a cursor for each element whose kind
// is in kinds. It is [Preorder] over the same content: the same elements in the
// same order, the same cursors, the same break and [Cursor.SkipChildren]. What
// it does differently is invisible — a subtree holding none of the kinds asked
// for is stepped over rather than walked through.
func (x *Index) Preorder(kinds KindSet) iter.Seq[Cursor] {
	return func(yield func(Cursor) bool) {
		w := &walker{yield: yield, kinds: kinds}
		w.stack = w.inline[:0]
		for i := 0; i < len(x.nodes); {
			n := &x.nodes[i]
			if n.subtree&kinds == 0 {
				i = int(n.end)
				continue
			}
			// Everything still open below this element's depth is behind us.
			for len(w.stack) > int(n.depth) {
				w.pop()
			}
			w.push(n.c)
			if kinds.Contains(n.kind) && !w.visit() {
				if !w.live() {
					return
				}
				i = int(n.end) // pruned by the caller
				continue
			}
			i++
		}
	}
}
