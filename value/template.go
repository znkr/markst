package value

import (
	"znkr.io/writst/internal/formatter"
	"znkr.io/writst/name"
	"znkr.io/writst/types"
)

// This file implements the value-layer machinery for `#set` and `#show` rules.
// The design is deferred: a rule is *recorded* in the content tree rather than
// folded into element fields. There is no realize/render pass yet, so the
// styling effect is not observable in golden output; only the structure is.
//
// Element, Set, Recipe, and the Selector variants are plain values. The two
// content nodes — TemplateUpdate and Templated — are hand-written like
// [StateUpdate] and opt out of the fieldaccessgen generator.

// Element is a set/show target such as `heading` or `text`. An element *is* a
// function — it shares [Function]'s shape and doubles as its own constructor —
// but is a distinct type so the evaluator can tell set/show/`.where` targets
// apart from ordinary functions. It reports [types.Function] so
// `type(heading) == function` (matching Typst). Convert with
// `(*Function)(elem)` to call it.
type Element Function

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

// BindTemplateSet validates a set-rule's arguments against the element's own
// signature and, on success, collects the named arguments into a [Set]. It uses
// [Function.bind] — which rejects unknown arguments and type mismatches but,
// unlike [Function.Apply], does not require the missing parameters — because a
// set rule overrides only a subset of an element's properties. Positional
// arguments are dropped from the recorded [Set]: an element's properties are its
// named parameters.
func (n *Element) BindTemplateSet(args *Arguments) (*Set, error) {
	if _, _, err := (*Function)(n).bind(args); err != nil {
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
// `show: transform`) and the transform value applied to matching content.
// Transform is stored, not applied — it is a *Function, Content, *Element, or
// *TemplateUpdate.
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

// TemplateUpdate is the transient content node a set/show rule evaluates to. It
// rides the content sequence — analogous to [StateUpdate] — and is consumed by
// the assembly step (folded into a [Templated] wrapper), never surviving into
// final output.
type TemplateUpdate struct {
	Set    *Set
	Recipe *Recipe
	Label  *Label
}

func (*TemplateUpdate) aValue()          {}
func (*TemplateUpdate) aContent()        {}
func (*TemplateUpdate) Type() types.Type { return types.Content }

func (n *TemplateUpdate) Equal(other Value) bool {
	o, ok := other.(*TemplateUpdate)
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

func (n *TemplateUpdate) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Str("template(")
	switch {
	case n.Set != nil:
		n.Set.Format(f)
	case n.Recipe != nil:
		n.Recipe.Format(f)
	}
	f.Str(")")
}

func (n *TemplateUpdate) Field(name.Name) Value   { return nil }
func (n *TemplateUpdate) HasField(name.Name) bool { return false }
func (n *TemplateUpdate) Fields() *Dict           { return new(Dict) }
func (n *TemplateUpdate) Name() string            { return "template" }
func (n *TemplateUpdate) SetLabel(label *Label) *Label {
	old := n.Label
	n.Label = label
	return old
}

// Templated is the reified per-scope template: one wrapper per set/show scope,
// recording the rules that apply to Body (the remaining siblings after the
// rule). Mirrors Typst's StyledElem. Rules are recorded, not applied — concrete
// resolution is a future realize pass.
type Templated struct {
	Sets    []*Set
	Recipes []*Recipe
	Body    Content
	Label   *Label
}

func (*Templated) aValue()          {}
func (*Templated) aContent()        {}
func (*Templated) Type() types.Type { return types.Content }

func (n *Templated) Equal(other Value) bool {
	o, ok := other.(*Templated)
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

func (n *Templated) Format(f *formatter.Formatter) {
	f.Prefix()
	f.Str("templated(")
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

func (n *Templated) Field(name.Name) Value   { return nil }
func (n *Templated) HasField(name.Name) bool { return false }
func (n *Templated) Fields() *Dict           { return new(Dict) }
func (n *Templated) Name() string            { return "templated" }
func (n *Templated) SetLabel(label *Label) *Label {
	old := n.Label
	n.Label = label
	return old
}
