package value

import (
	"znkr.io/writst/internal/formatter"
	"znkr.io/writst/internal/names"
	"znkr.io/writst/name"
	"znkr.io/writst/types"
)

// This file implements the value-layer machinery for `#set` and `#show` rules.
// During evaluation a rule is *recorded* in the content tree rather than folded
// into element fields; the realization pass (eval/realize.go) later resolves the
// wrappers — applying show recipes and hoisting `set document(title:)` — so they
// don't survive into final output.
//
// Element, Set, Recipe, and the Selector variants are plain values. The two
// content nodes — StyleUpdate and Styled — are hand-written like [StateUpdate]
// and opt out of the fieldaccessgen generator.

// Element is a constructor function for elements and a set/show target such as
// `heading` or `text`.
type Element struct {
	Function
	produces func(Content) bool
}

// NewElement builds an element whose constructor is fn and whose content is the
// Go type T. T is the single source of truth for what the element matches.
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

// BindStyleSet validates a set-rule's arguments against the element's own
// signature and, on success, collects the named arguments into a [Set]. It uses
// [Function.bind] — which rejects unknown arguments and type mismatches but,
// unlike [Function.Apply], does not require the missing parameters — because a
// set rule overrides only a subset of an element's properties. Positional
// arguments are dropped from the recorded [Set]: an element's properties are its
// named parameters.
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

// Set is one resolved set-rule effect: the element being styled and the
// property values it assigns.
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

// Recipe is one resolved show-rule effect: a selector (nil for a bare
// `show: transform`) and the transform applied to matching content by the
// realization pass. Transform is a *Function, Content, *Element, or
// *StyleUpdate.
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

// Selector identifies the content a show recipe applies to. Every selector is a
// [Value], so equality goes through the usual [Value.Equal].
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

// StyleUpdate is the transient content node a set/show rule evaluates to. It
// rides the content sequence — analogous to [StateUpdate] — and is consumed by
// the assembly step (folded into a [Styled] wrapper), never surviving into
// final output.
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

// Styled is the reified style scope: one wrapper per set/show scope,
// recording the rules that apply to Body (the remaining siblings after the
// rule). Mirrors Typst's StyledElem. The realization pass (eval/realize.go)
// resolves it — applying recipes to Body and dropping the wrapper.
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
