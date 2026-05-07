package value

import (
	"fmt"
	"slices"
	"unique"

	"znkr.io/writst/syntax"
	"znkr.io/writst/types"
)

// Function represents a Writst function value, encompassing both user-defined
// closures and built-in functions. It describes the parameter signature and
// holds the implementation.
type Function struct {
	// Name is the function's name.
	Name string

	// Positional describes the function's positional parameters in order.
	Positional []Param

	// Variadic, if set, describes the variadic parameter that collects any
	// surplus positional arguments.
	Variadic *Param

	// Named describes the allowed named parameters.
	Named NamedParams

	// WithArgs, if set, contains arguments that are pre-bound to the function
	// (partial application).
	WithArgs *Arguments

	// F is the function's implementation. It receives the call context and
	// fully merged arguments (pre-bound + call-site) and returns the result or
	// an error.
	F func(call *FunctionCallContext, args []Value, named NamedArgsWithDefaults) (Value, error)

	// Bind, if set, overrides the default argument validation and slot
	// mapping. It receives the merged arguments (WithArgs + call args) and
	// returns the validated arguments along with a positional slot mapping.
	// When set, the default type checking and slot assignment are skipped.
	Bind func(fn *Function, args *Arguments) (*Arguments, []int, error)
}

// NamedParams maps interned parameter names to their definitions.
type NamedParams map[unique.Handle[string]]Param

// Param describes a function parameter: its name, accepted types, and
// optional default value (nil means required).
type Param struct {
	Name    string
	Type    types.Set
	Default Value
}

// NamedArgsWithDefaults pairs call-site named arguments with the function's
// default values, providing [Get] and [IsSet] for convenient access.
type NamedArgsWithDefaults struct {
	Args     NamedArgs
	Defaults NamedParams
}

// IsSet reports whether the named argument was explicitly provided at the call site.
func (n *NamedArgsWithDefaults) IsSet(name unique.Handle[string]) bool {
	_, ok := n.Args[name]
	return ok
}

// Get returns the value of a named argument, falling back to its default
// value if not explicitly provided, or [None] if there is no default.
func (n *NamedArgsWithDefaults) Get(name unique.Handle[string]) Value {
	v := n.Args[name]
	if v == nil {
		var ok bool
		def, ok := n.Defaults[name]
		if ok {
			v = def.Default
		}
		if v == nil {
			v = None{}
		}
	}
	return v
}

// With returns a copy of the function with args pre-bound (partial application).
// This is used for method calls where the receiver is bound as the first
// positional argument.
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

// bind merges WithArgs with args, validates types and named arguments, and
// returns the merged arguments along with a parameter slot mapping.
//
// mapping[paramIndex] is the arg index that fills that slot, or -1 if the slot
// uses its default, or -2 if the slot is required but unfilled.
func (n *Function) bind(args *Arguments) (*Arguments, []int, error) {
	merged := n.WithArgs.Merge(args)
	if n.Bind != nil {
		return n.Bind(n, merged)
	}

	m := len(merged.Positional)
	if n.Variadic == nil && m > len(n.Positional) {
		return nil, nil, ArgErrorPosf(len(n.Positional), "unexpected argument")
	}

	// mapping[paramIndex]: arg index that fills the slot, -1 for default, -2
	// for unset.
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
			// Skip optional slots when remaining args can only cover required
			// slots.
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

// FunctionCallContext carries call-site information passed to function
// implementations.
type FunctionCallContext struct {
	Span syntax.Span

	// Setter can be set by the function to support assignment to the result of
	// a function call, e.g. array.at(). This is a bit of a hack and there's
	// probably better ways to support this, but it works for now.
	Setter *func(Value)
}

// Apply calls the function with the given arguments. It merges WithArgs,
// validates and maps positional arguments to parameter slots, fills defaults
// for optional parameters, and invokes F.
func (n *Function) Apply(call *FunctionCallContext, args *Arguments) (Value, error) {
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
