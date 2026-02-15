package ir

import (
	"fmt"
	"maps"
	"slices"
	"unique"

	"github.com/woodsbury/decimal128"
	"znkr.io/writst/ir/types"
	"znkr.io/writst/syntax"
)

type Value interface {
	Type() types.Type

	aValue()
	formattable
}

// Type ////////////////////////////////////////////////////////////////////////////////////////////

type Type struct {
	Reflected   types.Type
	Constructor *Function
}

func (*Type) aValue()          {}
func (*Type) Type() types.Type { return types.ReflectedType }

// Scalars /////////////////////////////////////////////////////////////////////////////////////////

type None struct{}
type Auto struct{}
type Bool bool
type Int int64
type Float float64
type Decimal decimal128.Decimal
type Str string
type Bytes string

type Numeric struct {
	Value float64
	Unit  Unit
}

func (None) aValue()    {}
func (Auto) aValue()    {}
func (Bool) aValue()    {}
func (Int) aValue()     {}
func (Float) aValue()   {}
func (Decimal) aValue() {}
func (Str) aValue()     {}
func (Bytes) aValue()   {}
func (Numeric) aValue() {}

func (None) Type() types.Type    { return types.None }
func (Auto) Type() types.Type    { return types.Auto }
func (Bool) Type() types.Type    { return types.Bool }
func (Int) Type() types.Type     { return types.Int }
func (Float) Type() types.Type   { return types.Float }
func (Decimal) Type() types.Type { return types.Decimal }
func (Str) Type() types.Type     { return types.Str }
func (Bytes) Type() types.Type   { return types.Bytes }

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
type Dict map[Str]Value

func (Array) aValue() {}
func (Dict) aValue()  {}

func (Array) Type() types.Type { return types.Array }
func (Dict) Type() types.Type  { return types.Dict }

// Functions ///////////////////////////////////////////////////////////////////////////////////////

type Function struct {
	Name       string
	Positional []types.Set // allowed types for each positional argument
	Named      NamedParams // allowed named arguments (with default values)
	WithArgs   *Arguments
	F          func(call *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error)
}

type NamedParams map[unique.Handle[string]]NamedParam

type NamedParam struct {
	Type    types.Set
	Default Value
}

type NamedArgsWithDefaults struct {
	Args     NamedArgs
	Defaults NamedParams
}

func (n *NamedArgsWithDefaults) IsSet(name unique.Handle[string]) bool {
	_, ok := n.Args[name]
	return ok
}

func (n *NamedArgsWithDefaults) Get(name unique.Handle[string]) Value {
	v := n.Args[name]
	if v == nil {
		var ok bool
		def, ok := n.Defaults[name]
		if ok {
			v = def.Default
		}
		if v == nil {
			v = none
		}
	}
	return v
}

func (n *Function) With(args *Arguments) (*Function, error) {
	if err := n.validate(args, false); err != nil {
		return nil, err
	}
	return &Function{
		Name:       n.Name,
		Positional: n.Positional,
		WithArgs:   n.WithArgs.merge(args),
		Named:      n.Named,
		F:          n.F,
	}, nil
}

type FuncCallContext struct {
	Span syntax.Span
}

func (n *Function) Apply(call *FuncCallContext, args *Arguments) (Value, error) {
	if err := n.validate(args, true); err != nil {
		return nil, err
	}
	args = n.WithArgs.merge(args)
	if len(args.Positional) < len(n.Positional) {
		pos := make([]Value, len(n.Positional))
		copy(pos, args.Positional)
		for i := len(args.Positional); i < len(n.Positional); i++ {
			pos[i] = none
		}
		args.Positional = pos
	}
	return n.F(call, args.Positional, NamedArgsWithDefaults{
		Args:     args.Named,
		Defaults: n.Named,
	})
}

func (n *Function) validate(args *Arguments, strict bool) error {
	remaining := n.Positional
	if n.WithArgs != nil {
		remaining = remaining[len(n.WithArgs.Positional):]
	}
	if len(args.Positional) > len(remaining) {
		return fmt.Errorf("too many positional arguments: expected at most %d, got %d", len(n.Positional), len(args.Positional))
	}
	for i, ts := range remaining {
		if len(args.Positional) <= i && (!strict || ts.Contains(types.None)) {
			break
		}
		if len(args.Positional) <= i {
			return fmt.Errorf("too few positional arguments: expected at least %d, got %d", i+1, len(args.Positional))
		}
		if typ := args.Positional[i].Type(); !ts.Contains(typ) {
			return ArgErrorPosf(i, "expected %s, found %s", ts, typ)
		}
	}
	for name := range args.Named {
		if _, ok := n.Named[name]; !ok {
			return ArgErrorNamedf(name, "unknown named argument")
		}
		if typ := args.Named[name].Type(); !n.Named[name].Type.Contains(typ) {
			return ArgErrorNamedf(name, "expected %s, found %s", n.Named[name].Type, typ)
		}
	}
	return nil
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
