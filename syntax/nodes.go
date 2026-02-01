package syntax

import (
	"strings"
)

type Node struct {
	Kind  Kind
	Span  Span
	Value Value
}

type RootNode struct {
	Source Source
	Node
}

func (n Node) AsLeaf() *LeafValue {
	if v, ok := n.Value.(*LeafValue); ok {
		return v
	}
	return nil
}

func (n Node) AsInner() *InnerValue {
	if v, ok := n.Value.(*InnerValue); ok {
		return v
	}
	return nil
}

func (n Node) AsError() *ErrorValue {
	if v, ok := n.Value.(*ErrorValue); ok {
		return v
	}
	return nil
}

type Value interface {
	Text() string
	aValue()
}

type LeafValue struct {
	Literal string
}

func Leaf(kind Kind, span Span, literal string) Node {
	return Node{kind, span, &LeafValue{Literal: literal}}
}

func (v *LeafValue) Text() string {
	return v.Literal
}

func (v *LeafValue) aValue() {}

type InnerValue struct {
	Children []Node
}

func Inner(kind Kind, span Span, children []Node) Node {
	return Node{kind, span, &InnerValue{Children: children}}
}

func (v *InnerValue) Text() string {
	var sb strings.Builder
	for _, child := range v.Children {
		sb.WriteString(child.Value.Text())
	}
	return sb.String()
}

func (v *InnerValue) aValue() {}

type ErrorValue struct {
	Message string
	Hints   []string
	Literal string
}

func Error(msg string, span Span, literal string) Node {
	return Node{KindError, span, &ErrorValue{Message: msg, Literal: literal}}
}

func (v *ErrorValue) AddHint(hint string) {
	v.Hints = append(v.Hints, hint)
}

func (v *ErrorValue) Text() string {
	return v.Literal
}

func (v *ErrorValue) aValue() {}
