package value

import (
	"znkr.io/markst/internal/formatter"
	"znkr.io/markst/internal/names"
	"znkr.io/markst/name"
	"znkr.io/markst/types"
)

// This file holds the value types behind `#set` and `#show`.
//
// Evaluating a rule records it in the content tree instead of applying it. The
// realization pass in eval/realize.go resolves the recorded rules afterwards,
// so none of these types reach the output.
//
// [StyleUpdate] and [Styled] are content and are written by hand, not by the
// fieldaccessgen generator. Everything else here is an ordinary value.

// Element is the constructor function for one kind of content, such as
// `heading` or `text`. It is also what a set or show rule names as its target.
type Element struct {
	Function
	produces func(Content) bool
}

// NewElement returns an element whose constructor is fn and whose content is
// the Go type T. Which content the element matches follows from T alone.
func NewElement[T Content](fn Function) *Element {
	return &Element{
		Function: fn,
		produces: func(c Content) bool { _, ok := c.(T); return ok },
	}
}

func (*Element) aValue()          {}
func (*Element) Type() types.Type { return types.Function }

func (n *Element) Equal(other Value) bool {
	// Elements are singletons registered once in the universe, so pointer
	// identity is exact element equality.
	o, ok := other.(*Element)
	return ok && n == o
}

func (n *Element) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Str(n.Name)
}

// produced reports whether content c is an instance of element n.
func (n *Element) produced(c Content) bool {
	return n.produces != nil && n.produces(c)
}

// BindStyleSet checks args against the element's signature and returns the
// named ones as a [Set]. Unknown arguments and type mismatches are errors, but
// missing ones are not: a set rule overrides some of an element's properties,
// not all of them. Positional arguments are dropped, since an element's
// properties are its named parameters.
func (n *Element) BindStyleSet(args *Arguments) (*Set, error) {
	if _, _, err := n.Function.bind(args); err != nil {
		return nil, err
	}
	set := &Set{Element: n}
	for k, v := range args.Named.All() {
		set.Fields.Put(k, v)
	}
	return set, nil
}

// Set is what a set rule does: the element it styles, and the properties it
// assigns.
type Set struct {
	Element *Element
	Fields  NamedArgs
}

func (n *Set) Format(f *formatter.Formatter) {
	f.Str("set ")
	f.Str(n.Element.Name)
	f.Str("(")
	i := 0
	for k, v := range n.Fields.All() {
		if i > 0 {
			f.Str(", ")
		}
		f.Str(k.String())
		f.Str(": ")
		v.Format(f)
		i++
	}
	f.Str(")")
}

func (n *Set) equal(o *Set) bool {
	if n.Element != o.Element || n.Fields.Len() != o.Fields.Len() {
		return false
	}
	for k, v := range n.Fields.All() {
		ov, ok := o.Fields.Get(k)
		if !ok || !v.Equal(ov) {
			return false
		}
	}
	return true
}

// Recipe is what a show rule does: which content it applies to, and what it
// replaces that content with. Selector is nil for a bare `show: transform`,
// which applies to everything. Transform is a [Function], [Content], [Element],
// or [StyleUpdate].
type Recipe struct {
	Selector  Selector
	Transform Value
}

func (n *Recipe) Format(f *formatter.Formatter) {
	f.Str("show")
	if n.Selector != nil {
		f.Str(" ")
		n.Selector.Format(f)
	}
	f.Str(": ")
	n.Transform.Format(f)
}

func (n *Recipe) equal(o *Recipe) bool {
	if (n.Selector == nil) != (o.Selector == nil) {
		return false
	}
	if n.Selector != nil && !n.Selector.Equal(o.Selector) {
		return false
	}
	return n.Transform.Equal(o.Transform)
}

// Selector picks out the content a show recipe applies to.
type Selector interface {
	Value
	aSelector()

	// Match reports whether v is content this selector matches.
	Match(v Value) bool
}

// contentHasLabel reports whether c carries the label l.
func contentHasLabel(c Content, l name.Name) bool {
	lbl, ok := c.Field(names.Label).(*Label)
	return ok && lbl.Name == l
}

// ElementSelector matches every instance of an element (`show heading: ...`).
type ElementSelector struct {
	Element *Element
}

func (*ElementSelector) aSelector()       {}
func (*ElementSelector) aValue()          {}
func (*ElementSelector) Type() types.Type { return types.Selector }
func (n *ElementSelector) Format(f *formatter.Formatter) {
	f.Str(n.Element.Name)
}
func (n *ElementSelector) Equal(other Value) bool {
	o, ok := other.(*ElementSelector)
	return ok && n.Element == o.Element
}
func (n *ElementSelector) Match(v Value) bool {
	c, ok := v.(Content)
	return ok && n.Element.produced(c)
}

// LabelSelector matches content carrying a label (`show <intro>: ...`).
type LabelSelector struct {
	Label name.Name
}

func (*LabelSelector) aSelector()       {}
func (*LabelSelector) aValue()          {}
func (*LabelSelector) Type() types.Type { return types.Selector }
func (n *LabelSelector) Format(f *formatter.Formatter) {
	f.Str("<")
	f.Str(n.Label.String())
	f.Str(">")
}
func (n *LabelSelector) Equal(other Value) bool {
	o, ok := other.(*LabelSelector)
	return ok && n.Label == o.Label
}
func (n *LabelSelector) Match(v Value) bool {
	c, ok := v.(Content)
	return ok && contentHasLabel(c, n.Label)
}

// WhereSelector matches an element carrying a label
// (`show heading.where(label: <intro>): ...`).
type WhereSelector struct {
	Element *Element
	Label   name.Name
}

func (*WhereSelector) aSelector()       {}
func (*WhereSelector) aValue()          {}
func (*WhereSelector) Type() types.Type { return types.Selector }
func (n *WhereSelector) Format(f *formatter.Formatter) {
	f.Str(n.Element.Name)
	f.Str(".where(label: <")
	f.Str(n.Label.String())
	f.Str(">)")
}
func (n *WhereSelector) Equal(other Value) bool {
	o, ok := other.(*WhereSelector)
	return ok && n.Element == o.Element && n.Label == o.Label
}
func (n *WhereSelector) Match(v Value) bool {
	c, ok := v.(Content)
	return ok && n.Element.produced(c) && contentHasLabel(c, n.Label)
}

// StyleUpdate is what a set or show rule evaluates to. It sits in the content
// sequence until realization folds it into a [Styled] wrapper, and never
// reaches the output.
type StyleUpdate struct {
	Set    *Set
	Recipe *Recipe
	Label  *Label
}

func (*StyleUpdate) aValue()          {}
func (*StyleUpdate) aContent()        {}
func (*StyleUpdate) Type() types.Type { return types.Content }

func (n *StyleUpdate) Equal(other Value) bool {
	o, ok := other.(*StyleUpdate)
	if !ok || !labelEqual(n.Label, o.Label) {
		return false
	}
	if (n.Set == nil) != (o.Set == nil) || (n.Recipe == nil) != (o.Recipe == nil) {
		return false
	}
	if n.Set != nil && !n.Set.equal(o.Set) {
		return false
	}
	if n.Recipe != nil && !n.Recipe.equal(o.Recipe) {
		return false
	}
	return true
}

func (n *StyleUpdate) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Str("style(")
	switch {
	case n.Set != nil:
		n.Set.Format(f)
	case n.Recipe != nil:
		n.Recipe.Format(f)
	}
	f.Str(")")
}

func (n *StyleUpdate) Field(name.Name) Value   { return nil }
func (n *StyleUpdate) HasField(name.Name) bool { return false }
func (n *StyleUpdate) Fields() *Dict           { return new(Dict) }
func (n *StyleUpdate) Name() string            { return "style" }
func (n *StyleUpdate) IsBlock() bool           { return false }
func (n *StyleUpdate) SetLabel(label *Label) *Label {
	old := n.Label
	n.Label = label
	return old
}
func (n *StyleUpdate) GetLabel() *Label {
	return n.Label
}

// Styled is a style scope: the set and show rules in force, wrapped around the
// content they apply to. Realization applies the rules to Body and drops the
// wrapper, so a Styled never reaches the output.
//
// Corresponds to StyledElem in Typst.
type Styled struct {
	Sets    []*Set
	Recipes []*Recipe
	Body    Content
	Label   *Label
}

func (*Styled) aValue()          {}
func (*Styled) aContent()        {}
func (*Styled) Type() types.Type { return types.Content }

func (n *Styled) Equal(other Value) bool {
	o, ok := other.(*Styled)
	if !ok || len(n.Sets) != len(o.Sets) || len(n.Recipes) != len(o.Recipes) || !labelEqual(n.Label, o.Label) {
		return false
	}
	for i := range n.Sets {
		if !n.Sets[i].equal(o.Sets[i]) {
			return false
		}
	}
	for i := range n.Recipes {
		if !n.Recipes[i].equal(o.Recipes[i]) {
			return false
		}
	}
	return contentEqual(n.Body, o.Body)
}

func (n *Styled) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Str("styled(")
	first := true
	sep := func() {
		if !first {
			f.Str(", ")
		}
		first = false
	}
	for _, s := range n.Sets {
		sep()
		s.Format(f)
	}
	for _, r := range n.Recipes {
		sep()
		r.Format(f)
	}
	sep()
	f.Str("body: ")
	contentBlock{n.Body}.Format(f)
	f.Str(")")
}

func (n *Styled) Field(name.Name) Value   { return nil }
func (n *Styled) HasField(name.Name) bool { return false }
func (n *Styled) Fields() *Dict           { return new(Dict) }
func (n *Styled) Name() string            { return "styled" }
func (n *Styled) IsBlock() bool           { return false }
func (n *Styled) SetLabel(label *Label) *Label {
	old := n.Label
	n.Label = label
	return old
}
func (n *Styled) GetLabel() *Label {
	return n.Label
}
