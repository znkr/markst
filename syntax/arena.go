package syntax

// Arena allocates syntax nodes in blocks rather than one at a time. A tree's
// nodes are built together and dropped together — a parse has no use for one
// node without the rest — so they can share an allocation.
//
// The zero Arena is ready to use. It is not safe for concurrent use, and it
// frees nothing before the whole block goes: a block stays alive as long as any
// node in it does, which for a syntax tree is until the tree itself is dropped.
//
// [NewLeaf], [NewInner] and [NewError] are the same nodes allocated singly, for
// a caller building a tree of its own rather than parsing one.
type Arena struct {
	leaves   []Leaf
	inners   []Inner
	errors   []Error
	children []Node
}

// blockSize is how many nodes a block holds. Every block is the same size: a
// block that ends up half used wastes a few kilobytes at most, and sizing them
// from the source would be guessing at a ratio that varies with what the
// document is made of.
const blockSize = 128

// Leaf returns a terminal node with the given kind, span, and literal text.
func (a *Arena) Leaf(kind Kind, span Span, literal string) *Leaf {
	if len(a.leaves) == cap(a.leaves) {
		// A fresh block rather than a longer one: growing would copy the
		// nodes already handed out, and the pointers to them point at the
		// block they were cut from.
		a.leaves = make([]Leaf, 0, blockSize)
	}
	a.leaves = append(a.leaves, Leaf{kind: kind, span: span, literal: literal})
	return &a.leaves[len(a.leaves)-1]
}

// Inner returns a non-terminal node with the given kind and children, which the
// node takes over: they must not be mutated afterwards, for the reason
// [NewInner] gives. [Arena.Nodes] is where to get a slice to fill in.
func (a *Arena) Inner(kind Kind, children []Node) *Inner {
	if len(a.inners) == cap(a.inners) {
		a.inners = make([]Inner, 0, blockSize)
	}
	a.inners = append(a.inners, Inner{kind: kind, span: spanOf(children), children: children})
	return &a.inners[len(a.inners)-1]
}

// Error returns an error node with the given span, diagnostic message, original
// literal text, and optional hints.
func (a *Arena) Error(span Span, message string, literal string, hints ...string) *Error {
	if len(a.errors) == cap(a.errors) {
		a.errors = make([]Error, 0, blockSize)
	}
	a.errors = append(a.errors, Error{span: span, message: message, hints: hints, literal: literal})
	return &a.errors[len(a.errors)-1]
}

// Nodes returns a slice of n nil nodes to fill in and hand to [Arena.Inner].
// Its capacity is its length, so appending to it copies rather than writing
// over the children of the node allocated after it.
func (a *Arena) Nodes(n int) []Node {
	if n == 0 {
		return nil
	}
	if n > cap(a.children)-len(a.children) {
		a.children = make([]Node, 0, max(blockSize, n))
	}
	start := len(a.children)
	a.children = a.children[:start+n]
	return a.children[start : start+n : start+n]
}

// CloneNodes returns a copy of nodes allocated in the arena, for an [Inner]
// whose children the caller assembled elsewhere.
func (a *Arena) CloneNodes(nodes []Node) []Node {
	out := a.Nodes(len(nodes))
	copy(out, nodes)
	return out
}

// Convert returns a copy of node with a different kind, allocated in the arena.
// It panics on an [Error] node, which must not be reinterpreted.
func (a *Arena) Convert(node Node, kind Kind) Node {
	switch v := node.(type) {
	case *Leaf:
		return a.Leaf(kind, v.span, v.literal)
	case *Inner:
		return a.Inner(kind, v.children)
	case *Error:
		panic("cannot convert error node")
	default:
		panic("unexpected node type")
	}
}
