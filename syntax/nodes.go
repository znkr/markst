package syntax

import (
	"fmt"
	"strings"
)

type Node interface {
	Kind() Kind
	Span() Span
	Text() string
	aNode()
}

type RootNode struct {
	Source Source
	*Inner
}

type Leaf struct {
	kind    Kind
	span    Span
	literal string
}

var _ Node = (*Leaf)(nil)

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

type Inner struct {
	kind     Kind
	children []Node
}

var _ Node = (*Inner)(nil)

func NewInner(kind Kind, children []Node) *Inner {
	return &Inner{
		kind:     kind,
		children: children,
	}
}
func (v *Inner) Kind() Kind { return v.kind }
func (v *Inner) Span() Span {
	if len(v.children) == 0 {
		return Span{}
	}
	return Span{
		Start: v.children[0].Span().Start,
		End:   v.children[len(v.children)-1].Span().End,
	}
}
func (v *Inner) Text() string {
	var sb strings.Builder
	for _, child := range v.children {
		sb.WriteString(child.Text())
	}
	return sb.String()
}
func (v *Inner) Children() []Node { return v.children }
func (v *Inner) aNode()           {}

type Error struct {
	span    Span
	message string
	hints   []string
	literal string
}

var _ Node = (*Error)(nil)
var _ error = (*Error)(nil)

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
