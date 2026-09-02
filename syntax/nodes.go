package syntax

import "fmt"

// Node is the interface implemented by all nodes in the untyped syntax tree.
// Every node carries a [Kind] that determines its role, a [Span] locating it
// in the source text, and the original source text via [Text].
//
// Text is bytes rather than a string, and for a [Leaf] it is a slice of the
// source rather than a copy of it: scanning a document produces one node per
// token, and a copy each would be an allocation each. A caller that needs a
// string converts one where it needs one — which for the analyzer is once, for
// the whole source, with every node's text a slice of that.
//
// There are three concrete implementations:
//   - [Leaf]: a terminal token (e.g. an identifier, keyword, or operator).
//   - [Inner]: a non-terminal node with child nodes (e.g. a function call
//     or heading).
//   - [Error]: an invalid token or construct, carrying a diagnostic message.
type Node interface {
	Kind() Kind
	Span() Span
	Text() []byte
	aNode()
}

// RootNode is the top-level node returned by [parser.Parse]. It embeds the root
// [Inner] node (always of [KindMarkup]) and carries the [Source] needed to
// convert byte offsets to line/column positions.
type RootNode struct {
	// Src is the source the tree was parsed from. Every node's text is a slice
	// of it, so a caller holding this can turn any span into text without
	// going back to the node.
	Src    []byte
	Source Source
	*Inner
}

// Leaf is a terminal node in the syntax tree, representing a single token
// produced by the scanner. It stores the token's kind, source span, and the
// literal text.
type Leaf struct {
	kind    Kind
	span    Span
	literal []byte
}

var _ Node = (*Leaf)(nil)

// NewLeaf creates a new terminal node with the given kind, span, and literal
// text.
func NewLeaf(kind Kind, span Span, literal []byte) *Leaf {
	return &Leaf{
		kind:    kind,
		span:    span,
		literal: literal,
	}
}
func (v *Leaf) Kind() Kind   { return v.kind }
func (v *Leaf) Span() Span   { return v.span }
func (v *Leaf) Text() []byte { return v.literal }
func (v *Leaf) aNode()       {}

// Inner is a non-terminal node in the syntax tree, containing an ordered
// sequence of child nodes. Its [Span] is derived from the spans of its first
// and last children. Its [Text] is the concatenation of all children's text.
type Inner struct {
	kind     Kind
	span     Span
	children []Node
}

var _ Node = (*Inner)(nil)

// NewInner creates a new non-terminal node with the given kind and children.
// The children must not be mutated afterwards: the node's span is derived from
// them once, here, rather than on every [Inner.Span] call — recomputing it would
// recurse into both the first and the last child, which costs 2^depth for a
// deeply nested tree.
func NewInner(kind Kind, children []Node) *Inner {
	return &Inner{
		kind:     kind,
		span:     spanOf(children),
		children: children,
	}
}

// spanOf is the span an inner node covers: from the start of its first child to
// the end of its last.
func spanOf(children []Node) Span {
	if len(children) == 0 {
		return Span{}
	}
	return Span{
		Start: children[0].Span().Start,
		End:   children[len(children)-1].Span().End,
	}
}
func (v *Inner) Kind() Kind { return v.kind }
func (v *Inner) Span() Span { return v.span }

// Text returns the source the node covers, assembled from its children. Unlike
// a [Leaf]'s, it is built on each call rather than being a slice of the source.
func (v *Inner) Text() []byte {
	var b []byte
	for _, child := range v.children {
		b = append(b, child.Text()...)
	}
	return b
}
func (v *Inner) Children() []Node { return v.children }
func (v *Inner) aNode()           {}

// Error is a syntax tree node representing invalid source text. It implements
// both [Node] (so it can appear in the tree) and the built-in error interface
// (so it can be used as a Go error value). Its [Kind] is always [KindError].
//
// Error nodes carry a diagnostic message and optional hints for the user.
// They are created by the scanner for invalid tokens and by the parser for
// structural errors (e.g. unclosed delimiters, unexpected tokens).
type Error struct {
	span    Span
	message string
	hints   []string
	literal []byte
}

var _ Node = (*Error)(nil)
var _ error = (*Error)(nil)

// NewError creates a new error node with the given span, diagnostic message,
// original literal text, and optional hints.
func NewError(span Span, message string, literal []byte, hints ...string) *Error {
	return &Error{
		span:    span,
		message: message,
		hints:   hints,
		literal: literal,
	}
}

func (n *Error) Span() Span       { return n.span }
func (n *Error) Kind() Kind       { return KindError }
func (n *Error) Text() []byte     { return n.literal }
func (n *Error) Message() string  { return n.message }
func (n *Error) Hint(hint string) { n.hints = append(n.hints, hint) }
func (n *Error) Hints() []string  { return n.hints }
func (n *Error) Error() string    { return n.message }
func (n *Error) aNode()           {}

// ConvertNode creates a copy of node with a different kind, preserving the
// span and text (for Leaf) or children (for Inner). It panics if node is an
// Error, since error nodes should not be reinterpreted.
func ConvertNode(node Node, kind Kind) Node {
	switch v := node.(type) {
	case *Leaf:
		return NewLeaf(kind, v.span, v.literal)
	case *Inner:
		return NewInner(kind, v.children)
	case *Error:
		panic("cannot convert error node")
	default:
		panic(fmt.Sprintf("unexpected node type: %T", node))
	}
}
