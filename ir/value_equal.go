package ir

import (
	"slices"

	"github.com/woodsbury/decimal128"
	"znkr.io/writst/syntax"
)

// Equal compares two values using the same promotion rules as the == operator
// in the language.
func Equal(a, b Value) bool {
	x, y := promote(syntax.Eq, a, b)
	return x.Equal(y)
}

func labelEqual(a, b *Label) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Name == b.Name
}

func contentEqual(a, b Content) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Equal(b)
}

// Type ////////////////////////////////////////////////////////////////////////////////////////////

func (n *Type) Equal(other Value) bool {
	o, ok := other.(*Type)
	return ok && n.Reflected == o.Reflected
}

// Scalars /////////////////////////////////////////////////////////////////////////////////////////

func (n None) Equal(other Value) bool {
	_, ok := other.(None)
	return ok
}

func (n Auto) Equal(other Value) bool {
	_, ok := other.(Auto)
	return ok
}

func (n Bool) Equal(other Value) bool {
	o, ok := other.(Bool)
	return ok && n == o
}

func (n Int) Equal(other Value) bool {
	o, ok := other.(Int)
	return ok && n == o
}

func (n Float) Equal(other Value) bool {
	o, ok := other.(Float)
	return ok && n == o
}

func (n Decimal) Equal(other Value) bool {
	o, ok := other.(Decimal)
	return ok && decimal128.Decimal(n).Equal(decimal128.Decimal(o))
}

func (n Str) Equal(other Value) bool {
	o, ok := other.(Str)
	return ok && n == o
}

func (n Bytes) Equal(other Value) bool {
	o, ok := other.(Bytes)
	return ok && n == o
}

func (n Ratio) Equal(other Value) bool {
	o, ok := other.(Ratio)
	return ok && n == o
}

func (n Fraction) Equal(other Value) bool {
	o, ok := other.(Fraction)
	return ok && n == o
}

func (n Length) Equal(other Value) bool {
	o, ok := other.(Length)
	return ok && n.Pt == o.Pt && n.Em == o.Em
}

func (n Relative) Equal(other Value) bool {
	o, ok := other.(Relative)
	return ok && n.Ratio == o.Ratio && n.Length == o.Length
}

func (n Angle) Equal(other Value) bool {
	o, ok := other.(Angle)
	return ok && n == o
}

// Collections /////////////////////////////////////////////////////////////////////////////////////

func (n *Array) Equal(other Value) bool {
	o, ok := other.(*Array)
	if !ok || len(n.Elems) != len(o.Elems) {
		return false
	}
	for i := range n.Elems {
		x, y := promote(syntax.Eq, n.Elems[i], o.Elems[i])
		if !x.Equal(y) {
			return false
		}
	}
	return true
}

func (n Dict) Equal(other Value) bool {
	o, ok := other.(*Dict)
	if !ok || n.Elems.Len() != o.Elems.Len() {
		return false
	}
	for k, v := range n.Elems.All() {
		ov, exists := o.Elems.Get(k)
		if !exists {
			return false
		}
		x, y := promote(syntax.Eq, v, ov)
		if !x.Equal(y) {
			return false
		}
	}
	return true
}

// Functions ///////////////////////////////////////////////////////////////////////////////////////

func (n *Function) Equal(other Value) bool {
	o, ok := other.(*Function)
	return ok && n == o
}

func (n *Arguments) Equal(other Value) bool {
	o, ok := other.(*Arguments)
	if !ok {
		return false
	}
	if len(n.Positional) != len(o.Positional) {
		return false
	}
	for i := range n.Positional {
		if !n.Positional[i].Equal(o.Positional[i]) {
			return false
		}
	}
	if len(n.Named) != len(o.Named) {
		return false
	}
	for k, v := range n.Named {
		ov, exists := o.Named[k]
		if !exists || !v.Equal(ov) {
			return false
		}
	}
	return true
}

// Label ///////////////////////////////////////////////////////////////////////////////////////////

func (n *Label) Equal(other Value) bool {
	o, ok := other.(*Label)
	return ok && n.Name == o.Name
}

// Content /////////////////////////////////////////////////////////////////////////////////////////

func (n *Sequence) Equal(other Value) bool {
	o, ok := other.(*Sequence)
	if !ok || len(n.Children) != len(o.Children) || !labelEqual(n.Label, o.Label) {
		return false
	}
	for i := range n.Children {
		if !n.Children[i].Equal(o.Children[i]) {
			return false
		}
	}
	return true
}

func (n *Heading) Equal(other Value) bool {
	o, ok := other.(*Heading)
	return ok && n.Depth == o.Depth && n.Body.Equal(o.Body) && labelEqual(n.Label, o.Label)
}

func (n *Text) Equal(other Value) bool {
	o, ok := other.(*Text)
	return ok && n.Text == o.Text && labelEqual(n.Label, o.Label)
}

func (n *Raw) Equal(other Value) bool {
	o, ok := other.(*Raw)
	return ok && n.Block == o.Block && n.Lang == o.Lang && slices.Equal(n.Lines, o.Lines)
}

func (n *Strong) Equal(other Value) bool {
	o, ok := other.(*Strong)
	return ok && n.Body.Equal(o.Body) && labelEqual(n.Label, o.Label)
}

func (n *Emph) Equal(other Value) bool {
	o, ok := other.(*Emph)
	return ok && n.Body.Equal(o.Body) && labelEqual(n.Label, o.Label)
}

func (n *Linebreak) Equal(other Value) bool {
	_, ok := other.(*Linebreak)
	return ok
}

func (n *Parbreak) Equal(other Value) bool {
	_, ok := other.(*Parbreak)
	return ok
}

func (n *Link) Equal(other Value) bool {
	o, ok := other.(*Link)
	return ok && n.Dest == o.Dest && n.Body.Equal(o.Body) && labelEqual(n.Label, o.Label)
}

func (n *Ref) Equal(other Value) bool {
	o, ok := other.(*Ref)
	return ok && n.Target == o.Target && contentEqual(n.Supplement, o.Supplement) && labelEqual(n.Label, o.Label)
}

func (n *List) Equal(other Value) bool {
	o, ok := other.(*List)
	if !ok || len(n.Children) != len(o.Children) || !labelEqual(n.Label, o.Label) {
		return false
	}
	for i := range n.Children {
		if !n.Children[i].Equal(o.Children[i]) {
			return false
		}
	}
	return true
}

func (n *ListItem) Equal(other Value) bool {
	o, ok := other.(*ListItem)
	return ok && n.Body.Equal(o.Body) && labelEqual(n.Label, o.Label)
}

func (n *Enum) Equal(other Value) bool {
	o, ok := other.(*Enum)
	if !ok || len(n.Children) != len(o.Children) || !labelEqual(n.Label, o.Label) {
		return false
	}
	for i := range n.Children {
		if !n.Children[i].Equal(o.Children[i]) {
			return false
		}
	}
	return true
}

func (n *EnumItem) Equal(other Value) bool {
	o, ok := other.(*EnumItem)
	return ok && n.Number == o.Number && n.Body.Equal(o.Body) && labelEqual(n.Label, o.Label)
}

func (n *Terms) Equal(other Value) bool {
	o, ok := other.(*Terms)
	if !ok || len(n.Children) != len(o.Children) || !labelEqual(n.Label, o.Label) {
		return false
	}
	for i := range n.Children {
		if !n.Children[i].Equal(o.Children[i]) {
			return false
		}
	}
	return true
}

func (n *TermItem) Equal(other Value) bool {
	o, ok := other.(*TermItem)
	return ok && n.Term.Equal(o.Term) && n.Description.Equal(o.Description) && labelEqual(n.Label, o.Label)
}
