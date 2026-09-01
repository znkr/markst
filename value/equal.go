package value

import (
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

// attrsEqual compares two optional attribute dicts. It exists rather than
// [optEqual] because a nil *Dict is a non-nil Value holding a nil pointer, and
// [Dict.Equal] takes its receiver by value.
func attrsEqual(a, b *Dict) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(b)
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

// optEqual compares two optional field values, where nil means "unset". An
// unset field is equal only to another unset field.
func optEqual(a, b Value) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(b)
}

// Type ////////////////////////////////////////////////////////////////////////

func (n *Type) Equal(other Value) bool {
	o, ok := other.(*Type)
	return ok && n.Reflected == o.Reflected
}

// Scalars /////////////////////////////////////////////////////////////////////

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

// Dates ///////////////////////////////////////////////////////////////////////

func (n Datetime) Equal(other Value) bool {
	o, ok := other.(Datetime)
	return ok && n.Kind == o.Kind && n.T.Equal(o.T)
}

func (n Duration) Equal(other Value) bool {
	o, ok := other.(Duration)
	// Both are in canonical form, so equal lengths are equal field by field.
	return ok && n == o
}

// Collections /////////////////////////////////////////////////////////////////

// argumentsArrayEqual reports whether a positional-only arguments value equals
// an array with the same elements (in order). An arguments value carrying named
// arguments never equals an array.
func argumentsArrayEqual(args *Arguments, arr *Array) bool {
	if args.Named.Len() != 0 || len(args.Positional) != len(arr.Elems) {
		return false
	}
	for i := range args.Positional {
		x, y := promote(syntax.Eq, args.Positional[i], arr.Elems[i])
		if !x.Equal(y) {
			return false
		}
	}
	return true
}

func (n *Array) Equal(other Value) bool {
	if a, ok := other.(*Arguments); ok {
		return argumentsArrayEqual(a, n)
	}
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

// Functions ///////////////////////////////////////////////////////////////////

func (n *Function) Equal(other Value) bool {
	o, ok := other.(*Function)
	return ok && n == o
}

func (n *Arguments) Equal(other Value) bool {
	// A positional-only arguments value compares equal to an array of the same
	// values (matching Typst, where e.g. a closure's `..sink` — itself an
	// arguments value — tests equal to the array of captured positionals).
	if a, ok := other.(*Array); ok {
		return argumentsArrayEqual(n, a)
	}
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
	if n.Named.Len() != o.Named.Len() {
		return false
	}
	for k, v := range n.Named.All() {
		ov, exists := o.Named.Get(k)
		if !exists || !v.Equal(ov) {
			return false
		}
	}
	return true
}

// Label ///////////////////////////////////////////////////////////////////////

func (n *Label) Equal(other Value) bool {
	o, ok := other.(*Label)
	return ok && n.Name == o.Name
}

// Content /////////////////////////////////////////////////////////////////////

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
	return ok && n.Block == o.Block && n.Lang == o.Lang && n.Text == o.Text
}

func (n *Strong) Equal(other Value) bool {
	o, ok := other.(*Strong)
	return ok && n.Body.Equal(o.Body) && labelEqual(n.Label, o.Label)
}

func (n *Emph) Equal(other Value) bool {
	o, ok := other.(*Emph)
	return ok && n.Body.Equal(o.Body) && labelEqual(n.Label, o.Label)
}

func (n *Underline) Equal(other Value) bool {
	o, ok := other.(*Underline)
	return ok && n.Body.Equal(o.Body) && labelEqual(n.Label, o.Label)
}

func (n *Par) Equal(other Value) bool {
	o, ok := other.(*Par)
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

func (n *HSpace) Equal(other Value) bool {
	o, ok := other.(*HSpace)
	return ok && n.Amount == o.Amount
}

func (n *SmartQuote) Equal(other Value) bool {
	o, ok := other.(*SmartQuote)
	return ok && n.Double == o.Double && labelEqual(n.Label, o.Label)
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

func (n *Equation) Equal(other Value) bool {
	o, ok := other.(*Equation)
	return ok && n.Block == o.Block && n.Body.Equal(o.Body) && labelEqual(n.Label, o.Label)
}

func (n *MathText) Equal(other Value) bool {
	o, ok := other.(*MathText)
	return ok && n.Text == o.Text &&
		optEqual(n.Bold, o.Bold) && optEqual(n.Italic, o.Italic) && optEqual(n.Variant, o.Variant) &&
		labelEqual(n.Label, o.Label)
}

func (n *MathOp) Equal(other Value) bool {
	o, ok := other.(*MathOp)
	return ok && n.Limits == o.Limits && n.Text.Equal(o.Text) && labelEqual(n.Label, o.Label)
}

func (n *MathAttach) Equal(other Value) bool {
	o, ok := other.(*MathAttach)
	return ok && n.Base.Equal(o.Base) && contentEqual(n.Top, o.Top) && contentEqual(n.Bottom, o.Bottom) && labelEqual(n.Label, o.Label)
}

func (n *MathFrac) Equal(other Value) bool {
	o, ok := other.(*MathFrac)
	return ok && n.Num.Equal(o.Num) && n.Denom.Equal(o.Denom) && labelEqual(n.Label, o.Label)
}

func (n *MathRoot) Equal(other Value) bool {
	o, ok := other.(*MathRoot)
	return ok && contentEqual(n.Index, o.Index) && n.Radicand.Equal(o.Radicand) && labelEqual(n.Label, o.Label)
}

func (n *MathPrimes) Equal(other Value) bool {
	o, ok := other.(*MathPrimes)
	return ok && n.Base.Equal(o.Base) && n.Count == o.Count && labelEqual(n.Label, o.Label)
}

func (n *MathAlignPoint) Equal(other Value) bool {
	o, ok := other.(*MathAlignPoint)
	return ok && labelEqual(n.Label, o.Label)
}

func (n *MathDelimited) Equal(other Value) bool {
	o, ok := other.(*MathDelimited)
	return ok && n.Open.Equal(o.Open) && n.Body.Equal(o.Body) && n.Close.Equal(o.Close) && labelEqual(n.Label, o.Label)
}

func (n *MathUnderline) Equal(other Value) bool {
	o, ok := other.(*MathUnderline)
	return ok && n.Body.Equal(o.Body) && labelEqual(n.Label, o.Label)
}

func (n *MathAccent) Equal(other Value) bool {
	o, ok := other.(*MathAccent)
	if !ok || n.Accent != o.Accent || n.Dotless != o.Dotless {
		return false
	}
	return n.Base.Equal(o.Base) && optEqual(n.Size, o.Size) && labelEqual(n.Label, o.Label)
}

func (n *MathCancel) Equal(other Value) bool {
	o, ok := other.(*MathCancel)
	return ok && n.Body.Equal(o.Body) && optEqual(n.Angle, o.Angle) && labelEqual(n.Label, o.Label)
}

func (n *MathVec) Equal(other Value) bool {
	o, ok := other.(*MathVec)
	return ok && contentSliceEqual(n.Children, o.Children) && labelEqual(n.Label, o.Label)
}

func (n *MathCases) Equal(other Value) bool {
	o, ok := other.(*MathCases)
	return ok && contentSliceEqual(n.Children, o.Children) && labelEqual(n.Label, o.Label)
}

func (n *MathMat) Equal(other Value) bool {
	o, ok := other.(*MathMat)
	if !ok || len(n.Rows) != len(o.Rows) || !labelEqual(n.Label, o.Label) {
		return false
	}
	for i := range n.Rows {
		if !contentSliceEqual(n.Rows[i], o.Rows[i]) {
			return false
		}
	}
	return true
}

// contentSliceEqual reports whether two content slices are element-wise equal.
func contentSliceEqual(a, b []Content) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !a[i].Equal(b[i]) {
			return false
		}
	}
	return true
}

func (n *Table) Equal(other Value) bool {
	o, ok := other.(*Table)
	return ok && n.Columns == o.Columns && contentSliceEqual(n.Children, o.Children) &&
		labelEqual(n.Label, o.Label)
}

func (n *TableHeader) Equal(other Value) bool {
	o, ok := other.(*TableHeader)
	return ok && contentSliceEqual(n.Children, o.Children) && labelEqual(n.Label, o.Label)
}

func (n *Footnote) Equal(other Value) bool {
	o, ok := other.(*Footnote)
	return ok && n.Body.Equal(o.Body) && labelEqual(n.Label, o.Label)
}

func (n *Image) Equal(other Value) bool {
	o, ok := other.(*Image)
	return ok && n.Path == o.Path && n.Alt == o.Alt && labelEqual(n.Label, o.Label)
}

func (n *HTMLElem) Equal(other Value) bool {
	o, ok := other.(*HTMLElem)
	return ok && n.Tag == o.Tag && n.Block == o.Block && attrsEqual(n.Attrs, o.Attrs) &&
		contentEqual(n.Body, o.Body) && labelEqual(n.Label, o.Label)
}

func (n *Metadata) Equal(other Value) bool {
	o, ok := other.(*Metadata)
	return ok && optEqual(n.Value, o.Value) && labelEqual(n.Label, o.Label)
}

func (n *Document) Equal(other Value) bool {
	o, ok := other.(*Document)
	return ok && n.Title == o.Title &&
		datetimeEqual(n.Date, o.Date) &&
		contentEqual(n.Body, o.Body)
}

// datetimeEqual compares two optional datetimes, either of which may be unset.
func datetimeEqual(a, b *Datetime) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
