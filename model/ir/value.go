package ir

import (
	"fmt"
	"maps"
	"slices"
	"unique"
)

type Value interface {
	Kind() Kind

	aValue()
	formattable
}

//go:generate go run  golang.org/x/tools/cmd/stringer -type Kind
type Kind int

const (
	KindNone Kind = iota
	KindAuto
	KindBool
	KindInt
	KindFloat
	KindNumeric
	KindString
	KindArray
	KindDict
	KindFunction
	KindArguments
	KindContent
)

// Scalars /////////////////////////////////////////////////////////////////////////////////////////

type None struct{}
type Auto struct{}
type Bool bool
type Int int
type Float float64
type Numeric struct {
	Value float64
	Unit  Unit
}
type String string

func (None) aValue()    {}
func (Auto) aValue()    {}
func (Bool) aValue()    {}
func (Int) aValue()     {}
func (Float) aValue()   {}
func (Numeric) aValue() {}
func (String) aValue()  {}

func (None) Kind() Kind    { return KindNone }
func (Auto) Kind() Kind    { return KindAuto }
func (Bool) Kind() Kind    { return KindBool }
func (Int) Kind() Kind     { return KindInt }
func (Float) Kind() Kind   { return KindFloat }
func (Numeric) Kind() Kind { return KindNumeric }
func (String) Kind() Kind  { return KindString }

// Collections /////////////////////////////////////////////////////////////////////////////////////

type Array []Value
type Dict map[String]Value

func (Array) aValue() {}
func (Dict) aValue()  {}

func (Array) Kind() Kind { return KindArray }
func (Dict) Kind() Kind  { return KindDict }

// Functions ///////////////////////////////////////////////////////////////////////////////////////

type Function struct {
	Name          string
	NumPositional int
	Defaults      *Arguments
	F             func(args *Arguments) Value
}

func (n *Function) With(args *Arguments) *Function {
	n.validate(args, false)
	numPositional := n.NumPositional
	if numPositional >= 0 {
		numPositional -= len(args.Positional)
	}
	return &Function{
		Name:          n.Name,
		NumPositional: numPositional,
		Defaults:      n.Defaults.merge(args),
	}
}

func (n *Function) Apply(args *Arguments) Value {
	n.validate(args, true)
	return n.F(n.Defaults.merge(args))
}

func (n *Function) validate(args *Arguments, strict bool) {
	if n.NumPositional >= 0 {
		if !strict && len(args.Positional) > n.NumPositional {
			panic(fmt.Sprintf("too many positional arguments: expected at most %d, got %d", n.NumPositional, len(args.Positional)))
		} else if strict && len(args.Positional) != n.NumPositional {
			panic(fmt.Sprintf("incorrect number of positional arguments: expected at most %d, got %d", n.NumPositional, len(args.Positional)))
		}
	}
	var named NamedArgs
	if n.Defaults != nil {
		named = n.Defaults.Named
	}
	for name := range args.Named {
		if _, ok := named[name]; !ok {
			panic(fmt.Sprintf("unexpected named argument: %s", name.Value()))
		}
	}
}

func (*Function) aValue()    {}
func (*Function) Kind() Kind { return KindFunction }

// Arguments ///////////////////////////////////////////////////////////////////////////////////////

type NamedArgs map[unique.Handle[string]]Value

type Arguments struct {
	Positional []Value
	Named      NamedArgs
}

func (a *Arguments) merge(args *Arguments) *Arguments {
	if args == nil {
		return a
	}
	if a == nil {
		return args
	}

	var pos []Value
	if len(a.Positional) == 0 {
		pos = args.Positional
	} else if len(args.Positional) == 0 {
		pos = a.Positional
	} else {
		pos = slices.Concat(a.Positional, args.Positional)
	}

	var named NamedArgs
	if len(a.Named) == 0 {
		named = args.Named
	} else if len(args.Named) == 0 {
		named = a.Named
	} else {
		named = maps.Clone(a.Named)
		maps.Copy(named, args.Named)
	}
	return &Arguments{
		Positional: pos,
		Named:      named,
	}
}

func (*Arguments) aValue()    {}
func (*Arguments) Kind() Kind { return KindArguments }

// Content /////////////////////////////////////////////////////////////////////////////////////////

type Content interface {
	Value
	aElement()
}

type Contents []Content

type Heading struct {
	Level int
	Body  Content
}

type Strong struct {
	Body Content
}

type Emph struct {
	Body Content
}

type Text struct {
	Value string
}

type Raw struct {
	Block bool
	Lang  string
	Lines []string
}

type Linebreak struct{}

type Parbreak struct{}

type Link struct {
	Dest string
	Body Content
}

type Ref struct {
	Target     unique.Handle[string]
	Supplement Content
}

type List struct {
	Items []ListItem
}

type ListItem struct {
	Body Content
}

type Enum struct {
	Items []EnumItem
}

type EnumItem struct {
	Number int
	Body   Content
}

type Terms struct {
	Items []TermItem
}

type TermItem struct {
	Term        Content
	Description Content
}

func (Contents) aValue()   {}
func (*Heading) aValue()   {}
func (*Text) aValue()      {}
func (*Raw) aValue()       {}
func (*Strong) aValue()    {}
func (*Emph) aValue()      {}
func (*Linebreak) aValue() {}
func (*Parbreak) aValue()  {}
func (*Link) aValue()      {}
func (*Ref) aValue()       {}
func (*List) aValue()      {}
func (*ListItem) aValue()  {}
func (*Enum) aValue()      {}
func (*EnumItem) aValue()  {}
func (*Terms) aValue()     {}
func (*TermItem) aValue()  {}

func (Contents) aElement()   {}
func (*Heading) aElement()   {}
func (*Text) aElement()      {}
func (*Raw) aElement()       {}
func (*Strong) aElement()    {}
func (*Emph) aElement()      {}
func (*Linebreak) aElement() {}
func (*Parbreak) aElement()  {}
func (*Link) aElement()      {}
func (*Ref) aElement()       {}
func (*List) aElement()      {}
func (*ListItem) aElement()  {}
func (*Enum) aElement()      {}
func (*EnumItem) aElement()  {}
func (*Terms) aElement()     {}
func (*TermItem) aElement()  {}

func (Contents) Kind() Kind   { return KindContent }
func (*Heading) Kind() Kind   { return KindContent }
func (*Text) Kind() Kind      { return KindContent }
func (*Raw) Kind() Kind       { return KindContent }
func (*Strong) Kind() Kind    { return KindContent }
func (*Emph) Kind() Kind      { return KindContent }
func (*Linebreak) Kind() Kind { return KindContent }
func (*Parbreak) Kind() Kind  { return KindContent }
func (*Link) Kind() Kind      { return KindContent }
func (*Ref) Kind() Kind       { return KindContent }
func (*List) Kind() Kind      { return KindContent }
func (*ListItem) Kind() Kind  { return KindContent }
func (*Enum) Kind() Kind      { return KindContent }
func (*EnumItem) Kind() Kind  { return KindContent }
func (*Terms) Kind() Kind     { return KindContent }
func (*TermItem) Kind() Kind  { return KindContent }

// Label ///////////////////////////////////////////////////////////////////////////////////////////

type Label struct {
	Name unique.Handle[string]
}

func (Label) aValue()     {}
func (*Label) aElement()  {}
func (*Label) Kind() Kind { return KindContent }
