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

import "fmt"

// Node is implemented by every node in the untyped syntax tree.
//
// A node holds no text of its own. Its text is exactly the source it spans, so
// [Text] reads the text off the source instead. That keeps a node down to a
// kind and a span, which matters because parsing a document produces one node
// per token.
//
// There are three implementations:
//   - [Leaf]: a single token, such as an identifier, keyword, or operator.
//   - [Inner]: a node with children, such as a function call or a heading.
//   - [Error]: invalid source, carrying a diagnostic message.
type Node interface {
	// Kind returns what this node is.
	Kind() Kind

	// Span returns the range of source the node covers.
	Span() Span

	aNode()
}

// Text returns the slice of src that n covers. src must be the source n was
// parsed from.
func Text(src []byte, n Node) []byte {
	span := n.Span()
	return src[span.Start:span.End]
}

// RootNode is a parsed document: the tree, plus the source it was parsed from.
// The embedded [Inner] is the root node, always of [KindMarkup]. Carrying the
// source along means a caller holding a RootNode can turn any span in the tree
// into text or a line and column.
type RootNode struct {
	Src    []byte
	Source Source
	*Inner
}

// Leaf is a node with no children: one token, as the scanner produced it.
type Leaf struct {
	kind Kind
	span Span
}

var _ Node = (*Leaf)(nil)

// NewLeaf returns a leaf node of the given kind covering span.
func NewLeaf(kind Kind, span Span) *Leaf {
	return &Leaf{kind: kind, span: span}
}
func (v *Leaf) Kind() Kind { return v.kind }
func (v *Leaf) Span() Span { return v.span }
func (v *Leaf) aNode()     {}

// Inner is a node with children. Its [Span] runs from the start of its first
// child to the end of its last, which is the source it covers.
type Inner struct {
	kind     Kind
	span     Span
	children []Node
}

var _ Node = (*Inner)(nil)

// NewInner returns a node of the given kind with the given children, which
// must not be empty; use [NewEmptyInner] for a node with none. The node takes
// ownership of the slice, and its span is fixed at this point, so the children
// must not change afterwards.
func NewInner(kind Kind, children []Node) *Inner {
	// The span is derived once here rather than on every Span call: walking to
	// the first and last child on demand would recurse into both, costing
	// 2^depth on a deeply nested tree.
	return &Inner{
		kind:     kind,
		span:     spanOf(children),
		children: children,
	}
}

// NewEmptyInner returns a node of the given kind with no children, spanning
// nothing at offset pos.
func NewEmptyInner(kind Kind, pos uint32) *Inner {
	return &Inner{kind: kind, span: Span{Start: pos, End: pos}}
}

// spanOf is the span an inner node covers, from the start of its first child to
// the end of its last. It panics when there are no children, since there is
// then no span to derive: a childless node must be built by [NewEmptyInner] or
// [Arena.EmptyInner], which take the offset explicitly. Returning Span{0, 0}
// instead would place the node at the start of the file, and any parent whose
// last child it is would end up with a span that runs backwards.
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

// Children returns the node's children, in source order. The slice belongs to
// the node and must not be modified.
func (v *Inner) Children() []Node { return v.children }

func (v *Inner) aNode() {}

// Error is a node standing for invalid source. Its [Kind] is always
// [KindError], and it is a Go error as well as a [Node], so it can both sit in
// the tree and be returned.
//
// The scanner creates one for an invalid token, the parser for a structural
// problem such as an unclosed delimiter.
type Error struct {
	span    Span
	message string
	hints   []string
}

var _ Node = (*Error)(nil)
var _ error = (*Error)(nil)

// NewError returns an error node covering span, reporting message with the
// given hints.
func NewError(span Span, message string, hints ...string) *Error {
	return &Error{span: span, message: message, hints: hints}
}

func (n *Error) Span() Span { return n.span }
func (n *Error) Kind() Kind { return KindError }

// Message returns the diagnostic message, the same string [Error.Error]
// returns.
func (n *Error) Message() string { return n.message }

// Hint adds a hint to the diagnostic, shown alongside the message.
func (n *Error) Hint(hint string) { n.hints = append(n.hints, hint) }

// Hints returns the hints added so far, in the order they were added.
func (n *Error) Hints() []string { return n.hints }

// Error returns the diagnostic message.
func (n *Error) Error() string { return n.message }

func (n *Error) aNode() {}

// ConvertNode returns a copy of node with a different kind, keeping its span
// and, for an [Inner], its children. It panics if node is an [Error]: what an
// error node covers is not a construct of some other kind.
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
