package ir

//go:generate go tool znkr.io/writst/ir/internal/fieldaccessgen

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
	"unique"

	"github.com/woodsbury/decimal128"
	"znkr.io/writst/ir/types"
	"znkr.io/writst/syntax"
)

type Value interface {
	Type() types.Type
	Equal(Value) bool

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

type Ratio float64
type Fraction float64

type Length struct {
	Pt float64
	Em float64
}

type Relative struct {
	Ratio  Ratio
	Length Length
}
type Angle float64

func (n Ratio) String() string {
	return fmt.Sprintf("%g%%", float64(n)*100)
}

func (n Fraction) String() string {
	return fmt.Sprintf("%gfr", float64(n))
}

func (n Float) String() string {
	v := float64(n)
	switch {
	case math.IsInf(v, 1):
		return "inf"
	case math.IsInf(v, -1):
		return "-inf"
	case math.IsNaN(v):
		return "float.nan"
	default:
		return fmt.Sprintf("%g", n)
	}
}

func (n Length) String() string {
	if math.IsInf(n.Pt+n.Em, 1) {
		return "inf"
	}
	if math.IsInf(n.Pt+n.Em, -1) {
		return "-inf"
	}
	if math.IsNaN(n.Pt + n.Em) {
		return "nan"
	}
	var sb strings.Builder
	if n.Pt != 0 {
		fmt.Fprintf(&sb, "%gpt", n.Pt)
	}
	if n.Pt != 0 && n.Em != 0 {
		sb.WriteString(" + ")
	}
	if n.Em != 0 {
		fmt.Fprintf(&sb, "%gem", n.Em)
	}
	if sb.Len() == 0 {
		sb.WriteString("0pt")
	}
	return sb.String()
}

func (r Relative) String() string {
	return fmt.Sprintf("%g%% + %s", float64(r.Ratio)*100, r.Length)
}

func (n Angle) String() string {
	return fmt.Sprintf("%gdeg", float64(n*180/math.Pi))
}

func (None) aValue()     {}
func (Auto) aValue()     {}
func (Bool) aValue()     {}
func (Int) aValue()      {}
func (Float) aValue()    {}
func (Decimal) aValue()  {}
func (Str) aValue()      {}
func (Bytes) aValue()    {}
func (Ratio) aValue()    {}
func (Fraction) aValue() {}
func (Length) aValue()   {}
func (Relative) aValue() {}
func (Angle) aValue()    {}

func (None) Type() types.Type     { return types.None }
func (Auto) Type() types.Type     { return types.Auto }
func (Bool) Type() types.Type     { return types.Bool }
func (Int) Type() types.Type      { return types.Int }
func (Float) Type() types.Type    { return types.Float }
func (Decimal) Type() types.Type  { return types.Decimal }
func (Str) Type() types.Type      { return types.Str }
func (Bytes) Type() types.Type    { return types.Bytes }
func (Ratio) Type() types.Type    { return types.Ratio }
func (Fraction) Type() types.Type { return types.Fraction }
func (Length) Type() types.Type   { return types.Length }
func (Relative) Type() types.Type { return types.Relative }
func (Angle) Type() types.Type    { return types.Angle }

// Collections /////////////////////////////////////////////////////////////////////////////////////

type Array struct {
	Elems []Value
}
type Dict map[Str]Value

func (*Array) aValue() {}
func (Dict) aValue()   {}

func (*Array) Type() types.Type { return types.Array }
func (Dict) Type() types.Type   { return types.Dict }

// Functions ///////////////////////////////////////////////////////////////////////////////////////

type Function struct {
	Name       string
	Positional []Param     // allowed types for each positional argument
	Variadic   *Param      // if set, collects remaining positional args into an *Array
	Named      NamedParams // allowed named arguments (with default values)
	WithArgs   *Arguments
	F          func(call *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error)

	// Bind, if set, overrides the default argument validation and slot
	// mapping. It receives the merged arguments (WithArgs + call args) and
	// returns the validated arguments along with a positional slot mapping.
	// When set, the default type checking and slot assignment are skipped.
	Bind func(fn *Function, args *Arguments) (*Arguments, []int, error)
}

type NamedParams map[unique.Handle[string]]Param

type Param struct {
	Name    string
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
	merged, _, err := n.bind(args)
	if err != nil {
		return nil, err
	}
	return &Function{
		Name:       n.Name,
		Positional: n.Positional,
		Variadic:   n.Variadic,
		Named:      n.Named,
		WithArgs:   merged,
		F:          n.F,
		Bind:       n.Bind,
	}, nil
}

// bind merges WithArgs with args, validates types and named arguments, and returns the merged
// arguments along with a parameter slot mapping.
//
// mapping[paramIndex] is the arg index that fills that slot, or -1 if the slot uses its default, or
// -2 if the slot is required but unfilled.
func (n *Function) bind(args *Arguments) (*Arguments, []int, error) {
	merged := n.WithArgs.merge(args)
	if n.Bind != nil {
		return n.Bind(n, merged)
	}

	m := len(merged.Positional)
	if n.Variadic == nil && m > len(n.Positional) {
		return nil, nil, fmt.Errorf("too many positional arguments: expected at most %d, got %d", len(n.Positional), m)
	}

	// mapping[paramIndex]: arg index that fills the slot, -1 for default, -2 for unset.
	mapping := slices.Repeat([]int{-2}, len(n.Positional))

	// Type-check each arg against the non-variadic parameter slots it could
	// reach. Walk left to right: the lowest matching slot for arg[i] anchors
	// arg[i+1]'s search (slots are always assigned in order). Optional slots
	// are only consumed when there are surplus args beyond what's needed for
	// the remaining required slots.
	lo := 0
	reqRemaining := 0 // required slots in [lo, len(Positional))
	for _, p := range n.Positional {
		if p.Default == nil {
			reqRemaining++
		}
	}

	// Determine how many args are available for non-variadic slots.
	nonVariadicArgs := min(m, len(n.Positional))

	for i := range nonVariadicArgs {
		arg := merged.Positional[i]
		hi := len(n.Positional) - nonVariadicArgs + i
		argsLeft := nonVariadicArgs - i // args left including this one
		matched := false
		for slot := lo; slot <= hi; slot++ {
			// Skip optional slots when remaining args can only cover required slots.
			if n.Positional[slot].Default != nil && argsLeft <= reqRemaining {
				continue
			}
			if n.Positional[slot].Type.Contains(arg.Type()) {
				for j := lo; j < slot; j++ {
					mapping[j] = -1
				}
				mapping[slot] = i
				if n.Positional[slot].Default == nil {
					reqRemaining--
				}
				lo = slot + 1
				matched = true
				break
			}
		}
		if !matched {
			return nil, nil, ArgErrorPosf(i, "expected %s, found %s", n.Positional[lo].Type, arg.Type())
		}
	}
	for i := lo; i < len(n.Positional); i++ {
		if n.Positional[i].Default != nil {
			mapping[i] = -1
		}
	}

	// Handle variadic args: type-check each remaining arg.
	if n.Variadic != nil {
		for i := nonVariadicArgs; i < m; i++ {
			if !n.Variadic.Type.Contains(merged.Positional[i].Type()) {
				return nil, nil, ArgErrorPosf(i, "expected %s, found %s", n.Variadic.Type, merged.Positional[i].Type())
			}
		}
	}

	for name := range merged.Named {
		if _, ok := n.Named[name]; !ok {
			return nil, nil, ArgErrorNamedPairf(name, "unexpected argument: %s", name.Value())
		}
		if typ := merged.Named[name].Type(); !n.Named[name].Type.Contains(typ) {
			return nil, nil, ArgErrorNamedf(name, "expected %s, found %s", n.Named[name].Type, typ)
		}
	}
	return merged, mapping, nil
}

type FuncCallContext struct {
	Span syntax.Span

	// setter can be set by the function to support assignment to the result of a function call,
	// e.g. array.at(). This is a bit of a hack and there's probably better ways to support this,
	// but it works for now.
	setter *setter
}

// Apply calls the function with the given arguments. It merges WithArgs,
// validates and maps positional arguments to parameter slots, fills defaults
// for optional parameters, and invokes F.
func (n *Function) Apply(call *FuncCallContext, args *Arguments) (Value, error) {
	merged, mapping, err := n.bind(args)
	if err != nil {
		return nil, err
	}

	// A nil mapping means args pass through directly (no slot assignment).
	if mapping == nil {
		return n.F(call, merged.Positional, NamedArgsWithDefaults{Args: merged.Named})
	}

	pos := make([]Value, len(mapping))
	for i, argIdx := range mapping {
		switch argIdx {
		case -2:
			return nil, fmt.Errorf("missing argument: %s", n.Positional[i].Name)
		case -1:
			pos[i] = n.Positional[i].Default
		default:
			pos[i] = merged.Positional[argIdx]
		}
	}

	// Append variadic args as an *Array.
	if n.Variadic != nil {
		var elems []Value
		variadicStart := min(len(merged.Positional), len(n.Positional))
		if variadicStart < len(merged.Positional) {
			elems = slices.Clone(merged.Positional[variadicStart:])
		}
		pos = append(pos, &Array{Elems: elems})
	}

	return n.F(call, pos, NamedArgsWithDefaults{
		Args:     merged.Named,
		Defaults: n.Named,
	})
}

func (*Function) aValue()          {}
func (*Function) Type() types.Type { return types.Function }

// Arguments ///////////////////////////////////////////////////////////////////////////////////////

type Arguments struct {
	Positional []Value
	Named      NamedArgs
}

func (n *Arguments) merge(args *Arguments) *Arguments {
	if args == nil {
		return n
	}
	if n == nil {
		return args
	}

	var pos []Value
	if len(n.Positional) == 0 {
		pos = args.Positional
	} else if len(args.Positional) == 0 {
		pos = n.Positional
	} else {
		pos = slices.Concat(n.Positional, args.Positional)
	}

	var named NamedArgs
	if len(n.Named) == 0 {
		named = args.Named
	} else if len(args.Named) == 0 {
		named = n.Named
	} else {
		named = maps.Clone(n.Named)
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

	SetLabel(*Label) *Label

	Field(unique.Handle[string]) Value
	HasField(unique.Handle[string]) bool
	Fields() Dict

	aContent()
}

type Sequence struct {
	Children []Content `writst:"required"`
	Label    *Label
}

type Heading struct {
	Depth int     `writst:"required"`
	Body  Content `writst:"required"`
	Label *Label
}

type Strong struct {
	Body  Content `writst:"required"`
	Label *Label
}

type Emph struct {
	Body  Content `writst:"required"`
	Label *Label
}

type Text struct {
	Text  string `writst:"required"`
	Label *Label
}

type Raw struct {
	Block bool `writst:"required"`
	Lang  string
	Lines []string `writst:"required"`
}

type Linebreak struct{}

type Parbreak struct{}

type Link struct {
	Dest  string  `writst:"required"`
	Body  Content `writst:"required"`
	Label *Label
}

type Ref struct {
	Target     unique.Handle[string] `writst:"required"`
	Supplement Content
	Label      *Label
}

type List struct {
	Children []*ListItem `writst:"required"`
	Label    *Label
}

type ListItem struct {
	Body  Content `writst:"required"`
	Label *Label
}

type Enum struct {
	Children []*EnumItem `writst:"required"`
	Label    *Label
}

type EnumItem struct {
	Number int
	Body   Content `writst:"required"`
	Label  *Label
}

type Terms struct {
	Children []*TermItem `writst:"required"`
	Label    *Label
}

type TermItem struct {
	Term        Content `writst:"required"`
	Description Content `writst:"required"`
	Label       *Label
}

func (*Sequence) aValue()  {}
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

func (Sequence) aContent()   {}
func (*Heading) aContent()   {}
func (*Text) aContent()      {}
func (*Raw) aContent()       {}
func (*Strong) aContent()    {}
func (*Emph) aContent()      {}
func (*Linebreak) aContent() {}
func (*Parbreak) aContent()  {}
func (*Link) aContent()      {}
func (*Ref) aContent()       {}
func (*List) aContent()      {}
func (*ListItem) aContent()  {}
func (*Enum) aContent()      {}
func (*EnumItem) aContent()  {}
func (*Terms) aContent()     {}
func (*TermItem) aContent()  {}

func (Sequence) Type() types.Type   { return types.Content }
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
func (*Label) Type() types.Type { return types.Label }
