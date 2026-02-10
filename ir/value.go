package ir

import (
	"fmt"
	"maps"
	"slices"
	"unique"

	"znkr.io/writst/ir/types"
)

type Value interface {
	Type() types.Type

	aValue()
	formattable
}

// Scalars /////////////////////////////////////////////////////////////////////////////////////////

type None struct{}
type Auto struct{}
type Bool bool
type Int int64
type Float float64
type String string

type Numeric struct {
	Value float64
	Unit  Unit
}

func (None) aValue()    {}
func (Auto) aValue()    {}
func (Bool) aValue()    {}
func (Int) aValue()     {}
func (Float) aValue()   {}
func (String) aValue()  {}
func (Numeric) aValue() {}

func (None) Type() types.Type   { return types.None }
func (Auto) Type() types.Type   { return types.Auto }
func (Bool) Type() types.Type   { return types.Bool }
func (Int) Type() types.Type    { return types.Int }
func (Float) Type() types.Type  { return types.Float }
func (String) Type() types.Type { return types.String }

func (n Numeric) Type() types.Type {
	switch n.Unit {
	case UnitPt, UnitMm, UnitCm, UnitIn, UnitEm:
		return types.Length
	case UnitDeg, UnitRad:
		return types.Angle
	case UnitPercent:
		return types.Ratio
	case UnitFr:
		return types.Fraction
	default:
		panic(fmt.Sprintf("invalid unit: %s", n.Unit))
	}
}

// Collections /////////////////////////////////////////////////////////////////////////////////////

type Array []Value
type Dict map[String]Value

func (Array) aValue() {}
func (Dict) aValue()  {}

func (Array) Type() types.Type { return types.Array }
func (Dict) Type() types.Type  { return types.Dict }

// Functions ///////////////////////////////////////////////////////////////////////////////////////

type Function struct {
	Name          string
	NumPositional int
	Defaults      NamedArgs
	WithArgs      *Arguments
	F             func(args []Value, named NamedArgsWithDefaults) (Value, error)
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
		WithArgs:      n.WithArgs.merge(args),
		Defaults:      n.Defaults,
	}
}

func (n *Function) Apply(args *Arguments) (Value, error) {
	n.validate(args, true)
	args = n.WithArgs.merge(args)
	return n.F(args.Positional, NamedArgsWithDefaults{
		Args:     args.Named,
		Defaults: n.Defaults,
	})
}

func (n *Function) validate(args *Arguments, strict bool) {
	if n.NumPositional >= 0 {
		if !strict && len(args.Positional) > n.NumPositional {
			panic(fmt.Sprintf("too many positional arguments: expected at most %d, got %d", n.NumPositional, len(args.Positional)))
		} else if strict && len(args.Positional) != n.NumPositional {
			panic(fmt.Sprintf("incorrect number of positional arguments: expected at most %d, got %d", n.NumPositional, len(args.Positional)))
		}
	}
	for name := range args.Named {
		if _, ok := n.Defaults[name]; !ok {
			panic(fmt.Sprintf("unexpected named argument: %s", name.Value()))
		}
	}
}

func (*Function) aValue()          {}
func (*Function) Type() types.Type { return types.Function }

// Arguments ///////////////////////////////////////////////////////////////////////////////////////

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

func (*Arguments) aValue()          {}
func (*Arguments) Type() types.Type { return types.Arguments }

type NamedArgs map[unique.Handle[string]]Value

type NamedArgsWithDefaults struct {
	Args     NamedArgs
	Defaults NamedArgs
}

func (n *NamedArgsWithDefaults) IsSet(name unique.Handle[string]) bool {
	_, ok := n.Args[name]
	return ok
}

func (n *NamedArgsWithDefaults) Get(name unique.Handle[string]) Value {
	v := n.Args[name]
	if v == nil {
		v = n.Defaults[name]
	}
	return v
}

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

func (Contents) Type() types.Type   { return types.Content }
func (*Heading) Type() types.Type   { return types.Content }
func (*Text) Type() types.Type      { return types.Content }
func (*Raw) Type() types.Type       { return types.Content }
func (*Strong) Type() types.Type    { return types.Content }
func (*Emph) Type() types.Type      { return types.Content }
func (*Linebreak) Type() types.Type { return types.Content }
func (*Parbreak) Type() types.Type  { return types.Content }
func (*Link) Type() types.Type      { return types.Content }
func (*Ref) Type() types.Type       { return types.Content }
func (*List) Type() types.Type      { return types.Content }
func (*ListItem) Type() types.Type  { return types.Content }
func (*Enum) Type() types.Type      { return types.Content }
func (*EnumItem) Type() types.Type  { return types.Content }
func (*Terms) Type() types.Type     { return types.Content }
func (*TermItem) Type() types.Type  { return types.Content }

// Label ///////////////////////////////////////////////////////////////////////////////////////////

type Label struct {
	Name unique.Handle[string]
}

func (Label) aValue()           {}
func (*Label) aElement()        {}
func (*Label) Type() types.Type { return types.Content }
