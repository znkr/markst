package value

import "iter"

// Tree is a walkable content tree: a [Document], or an [Index] over one. A
// function takes a Tree rather than a [Content] so that a caller holding an
// index can pass it instead. Both walk the same elements in the same order and
// differ only in cost.
type Tree interface {
	// Preorder walks the tree; see [Preorder] and [Index.Preorder].
	Preorder(kinds KindSet) iter.Seq[Cursor]
}

// Index is a flat record of a content tree in document order: one entry per
// element, holding the element, its depth, where its subtree ends, and the set
// of kinds in that subtree.
//
// [Index.Preorder] yields the same cursors in the same order as [Preorder], but
// skips a subtree containing none of the requested kinds instead of descending
// into it and filtering. Building the index costs one traversal, so it pays off
// when a document is walked more than once.
//
// An index describes the tree as it was when the index was built. A change to
// the tree is not detected: the index will then visit elements that have been
// removed and miss ones that have been added. Writing a field that carries no
// structure, such as a label or a heading's depth, is safe; adding, removing,
// or replacing content is not, and requires a new index.
//
// The zero Index is empty. Build one with [NewIndex], or fill one in place with
// [Index.Init].
type Index struct {
	nodes []indexNode
}

// indexNode is one element's record, 40 bytes.
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

// Init fills x with an index of c, reusing memory x has already allocated. Use
// it to hold one index across several documents.
func (x *Index) Init(c Content) {
	x.nodes = x.nodes[:0]
	// open holds the elements whose subtrees are still being read, outermost
	// first. An element is closed, fixing its end and its subtree's kinds, when
	// the walk returns to a depth at or above it.
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

// Preorder walks the indexed tree, yielding a cursor for each element whose
// kind is in kinds. It matches [Preorder] over the same content in every
// observable way: the same elements, the same order, the same cursors, and the
// same handling of break and [Cursor.SkipChildren]. It differs only in speed,
// by skipping subtrees that contain none of the requested kinds.
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
			// Any element open at or below this depth is complete.
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
