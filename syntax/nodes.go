package syntax

import (
	"fmt"
	"strings"
)

// Node is the interface implemented by all nodes in the untyped syntax tree.
// Every node carries a [Kind] that determines its role, a [Span] locating it
// in the source text, and the original source text via [Text].
//
// There are three concrete implementations:
//   - [Leaf]: a terminal token (e.g. an identifier, keyword, or operator).
//   - [Inner]: a non-terminal node with child nodes (e.g. a function call
//     or heading).
//   - [Error]: an invalid token or construct, carrying a diagnostic message.
type Node interface {
	Kind() Kind
	Span() Span
	Text() string
	aNode()
}

// RootNode is the top-level node returned by [parser.Parse]. It embeds the root
// [Inner] node (always of [KindMarkup]) and carries the [Source] needed to
// convert byte offsets to line/column positions.
type RootNode struct {
	Source Source
	*Inner
}

// Leaf is a terminal node in the syntax tree, representing a single token
// produced by the scanner. It stores the token's kind, source span, and the
// literal text.
type Leaf struct {
	kind    Kind
	span    Span
	literal string
}

var _ Node = (*Leaf)(nil)

// NewLeaf creates a new terminal node with the given kind, span, and literal
// text.
func NewLeaf(kind Kind, span Span, literal string) *Leaf {
	return &Leaf{
		kind:    kind,
		span:    span,
		literal: literal,
	}
}
func (v *Leaf) Kind() Kind   { return v.kind }
func (v *Leaf) Span() Span   { return v.span }
func (v *Leaf) Text() string { return v.literal }
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
	var span Span
	if len(children) > 0 {
		span = Span{
			Start: children[0].Span().Start,
			End:   children[len(children)-1].Span().End,
		}
	}
	return &Inner{
		kind:     kind,
		span:     span,
		children: children,
	}
}
func (v *Inner) Kind() Kind { return v.kind }
func (v *Inner) Span() Span { return v.span }
func (v *Inner) Text() string {
	var sb strings.Builder
	for _, child := range v.children {
		sb.WriteString(child.Text())
	}
	return sb.String()
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
	literal string
}

var _ Node = (*Error)(nil)
var _ error = (*Error)(nil)

// NewError creates a new error node with the given span, diagnostic message,
// original literal text, and optional hints.
func NewError(span Span, message string, literal string, hints ...string) *Error {
	return &Error{
		span:    span,
		message: message,
		hints:   hints,
		literal: literal,
	}
}

func (n *Error) Span() Span       { return n.span }
func (n *Error) Kind() Kind       { return KindError }
func (n *Error) Text() string     { return n.literal }
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

// ErrorList is a list of syntax errors that implements error as well.
type ErrorList []*Error

func (e ErrorList) Error() string {
	var sb strings.Builder
	for i, err := range e {
		if i > 0 {
			sb.WriteByte('\n')
		}
		sb.WriteString(err.Error())
	}
	return sb.String()
}
