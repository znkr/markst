package syntax

import "znkr.io/markst/internal/slab"

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
	leaves   slab.Of[Leaf]
	inners   slab.Of[Inner]
	errors   slab.Of[Error]
	children slab.Of[Node]
}

// Leaf returns a terminal node with the given kind and span.
func (a *Arena) Leaf(kind Kind, span Span) *Leaf {
	return a.leaves.New(Leaf{kind: kind, span: span})
}

// Inner returns a non-terminal node with the given kind and children, which must
// not be empty — use [Arena.EmptyInner] for that. The node takes the children
// over: they must not be mutated afterwards, for the reason [NewInner] gives.
// [Arena.Nodes] is where to get a slice to fill in.
func (a *Arena) Inner(kind Kind, children []Node) *Inner {
	return a.inners.New(Inner{kind: kind, span: spanOf(children), children: children})
}

// EmptyInner returns a non-terminal node with no children, spanning nothing at
// offset pos.
func (a *Arena) EmptyInner(kind Kind, pos uint32) *Inner {
	return a.inners.New(Inner{kind: kind, span: Span{Start: pos, End: pos}})
}

// Error returns an error node with the given span, diagnostic message, and
// optional hints.
func (a *Arena) Error(span Span, message string, hints ...string) *Error {
	return a.errors.New(Error{span: span, message: message, hints: hints})
}

// Nodes returns a slice of n nil nodes to fill in and hand to [Arena.Inner].
// Its capacity is its length, so appending to it copies rather than writing
// over the children of the node allocated next.
func (a *Arena) Nodes(n int) []Node {
	return a.children.Slice(n)
}

// CloneNodes returns a copy of nodes allocated in the arena, for an [Inner]
// whose children the caller assembled elsewhere.
func (a *Arena) CloneNodes(nodes []Node) []Node {
	out := a.Nodes(len(nodes))
	copy(out, nodes)
	return out
}

// Convert returns a copy of node with a different kind, allocated in the arena.
// The copy keeps the node's span and, for an [Inner], its children. It panics on
// an [Error] node, which must not be reinterpreted.
func (a *Arena) Convert(node Node, kind Kind) Node {
	switch v := node.(type) {
	case *Leaf:
		return a.Leaf(kind, v.span)
	case *Inner:
		return a.inners.New(Inner{kind: kind, span: v.span, children: v.children})
	case *Error:
		panic("cannot convert error node")
	default:
		panic("unexpected node type")
	}
}
