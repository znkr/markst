package syntax

import "fmt"

// Node is the interface implemented by all nodes in the untyped syntax tree.
// Every node carries a [Kind] that determines its role and a [Span] locating it
// in the source text.
//
// A node holds no text of its own: its text is the source it spans, so [Text]
// reads it off the source rather than every node carrying a copy or a slice of
// it. That keeps a leaf down to a kind and a span, which is what makes a tree
// of them cheap — scanning a document produces one per token.
//
// There are three concrete implementations:
//   - [Leaf]: a terminal token (e.g. an identifier, keyword, or operator).
//   - [Inner]: a non-terminal node with child nodes (e.g. a function call
//     or heading).
//   - [Error]: an invalid token or construct, carrying a diagnostic message.
type Node interface {
	Kind() Kind
	Span() Span
	aNode()
}

// Text returns the source n covers. A node's text is exactly the source it
// spans: the scanner reads every token out of src, and an inner node runs from
// the start of its first child to the end of its last.
func Text(src []byte, n Node) []byte {
	span := n.Span()
	return src[span.Start:span.End]
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
// produced by the scanner. It stores the token's kind and source span; its text
// is [Text] of that span.
type Leaf struct {
	kind Kind
	span Span
}

var _ Node = (*Leaf)(nil)

// NewLeaf creates a new terminal node with the given kind and span.
func NewLeaf(kind Kind, span Span) *Leaf {
	return &Leaf{kind: kind, span: span}
}
func (v *Leaf) Kind() Kind { return v.kind }
func (v *Leaf) Span() Span { return v.span }
func (v *Leaf) aNode()     {}

// Inner is a non-terminal node in the syntax tree, containing an ordered
// sequence of child nodes. Its [Span] runs from the start of its first child to
// the end of its last, which is also the source it covers.
type Inner struct {
	kind     Kind
	span     Span
	children []Node
}

var _ Node = (*Inner)(nil)

// NewInner creates a new non-terminal node with the given kind and children,
// which must not be empty — use [NewEmptyInner] for that. The children must not
// be mutated afterwards: the node's span is derived from them once, here, rather
// than on every [Inner.Span] call — recomputing it would recurse into both the
// first and the last child, which costs 2^depth for a deeply nested tree.
func NewInner(kind Kind, children []Node) *Inner {
	return &Inner{
		kind:     kind,
		span:     spanOf(children),
		children: children,
	}
}

// NewEmptyInner creates a new non-terminal node with no children, spanning
// nothing at offset pos.
func NewEmptyInner(kind Kind, pos uint32) *Inner {
	return &Inner{kind: kind, span: Span{Start: pos, End: pos}}
}

// spanOf is the span an inner node covers: from the start of its first child to
// the end of its last. It panics on no children, which have no span to derive:
// a childless node must be built by [NewEmptyInner] or [Arena.EmptyInner],
// which take the offset it sits at. Deriving Span{0, 0} instead would put the
// node at the start of the file and drag any parent whose last child it is down
// with it, leaving a parent span that runs backwards.
func spanOf(children []Node) Span {
	if len(children) == 0 {
		panic("syntax: inner node with no children has no span to derive")
	}
	return Span{
		Start: children[0].Span().Start,
		End:   children[len(children)-1].Span().End,
	}
}
func (v *Inner) Kind() Kind { return v.kind }
func (v *Inner) Span() Span { return v.span }

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
}

var _ Node = (*Error)(nil)
var _ error = (*Error)(nil)

// NewError creates a new error node with the given span, diagnostic message,
// and optional hints.
func NewError(span Span, message string, hints ...string) *Error {
	return &Error{span: span, message: message, hints: hints}
}

func (n *Error) Span() Span       { return n.span }
func (n *Error) Kind() Kind       { return KindError }
func (n *Error) Message() string  { return n.message }
func (n *Error) Hint(hint string) { n.hints = append(n.hints, hint) }
func (n *Error) Hints() []string  { return n.hints }
func (n *Error) Error() string    { return n.message }
func (n *Error) aNode()           {}

// ConvertNode creates a copy of node with a different kind, preserving its span
// and, for an [Inner], its children. It panics if node is an Error, since error
// nodes should not be reinterpreted.
func ConvertNode(node Node, kind Kind) Node {
	switch v := node.(type) {
	case *Leaf:
		return NewLeaf(kind, v.span)
	case *Inner:
		return &Inner{kind: kind, span: v.span, children: v.children}
	case *Error:
		panic("cannot convert error node")
	default:
		panic(fmt.Sprintf("unexpected node type: %T", node))
	}
}
