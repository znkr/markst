package value

import (
	"fmt"
	"slices"

	"znkr.io/writst/name"
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
	// If Sink is set, the parameter at index *Sink is the sink parameter
	// and receives a *Arguments value containing surplus positional args
	// and unmatched named args.
	Positional []Param

	// Sink, if set, marks Positional[*Sink] as a sink parameter. Pre-sink
	// params consume from the front of call args, post-sink params from the
	// back, and everything in between (plus unmatched named args) is packed
	// into a *Arguments passed at that index.
	Sink *int

	// Named describes the allowed named parameters.
	Named NamedParams

	// WithArgs, if set, contains arguments that are pre-bound to the function
	// (partial application).
	WithArgs *Arguments

	// F is the function's implementation. It receives the call context and
	// fully merged arguments (pre-bound + call-site) and returns the result or
	// an error.
	F func(call *FunctionCallContext, args []Value, named NamedArgsWithDefaults) (Value, error)
}

// NamedParams maps interned parameter names to their definitions.
type NamedParams map[name.Name]Param

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
func (n *NamedArgsWithDefaults) IsSet(name name.Name) bool {
	_, ok := n.Args[name]
	return ok
}

// Get returns the value of a named argument, falling back to its default
// value if not explicitly provided, or [None] if there is no default.
func (n *NamedArgsWithDefaults) Get(name name.Name) Value {
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
		Sink:       n.Sink,
		Named:      n.Named,
		WithArgs:   merged,
		F:          n.F,
	}, nil
}

// bind merges WithArgs with args, validates types and named arguments, and
// returns the merged arguments along with a parameter slot mapping.
//
// mapping[paramIndex] is the arg index that fills that slot, or -1 if the slot
// uses its default, or -2 if the slot is required but unfilled. The sink slot
// (if any) is set to -3; Apply handles it separately.
func (n *Function) bind(args *Arguments) (*Arguments, []int, error) {
	merged := n.WithArgs.Merge(args)

	sinkIdx := -1
	if n.Sink != nil {
		sinkIdx = *n.Sink
	}

	// Params split into pre-sink and post-sink groups (excluding the sink itself).
	var preSink, postSink []Param
	var preSinkSlots, postSinkSlots []int // original indices in Positional
	for i, p := range n.Positional {
		if i == sinkIdx {
			continue
		}
		if sinkIdx >= 0 && i > sinkIdx {
			postSink = append(postSink, p)
			postSinkSlots = append(postSinkSlots, i)
		} else {
			preSink = append(preSink, p)
			preSinkSlots = append(preSinkSlots, i)
		}
	}

	m := len(merged.Positional)
	nonSinkParamCount := len(preSink) + len(postSink)

	if sinkIdx < 0 && m > len(n.Positional) {
		return nil, nil, ArgErrorPosf(len(n.Positional), "unexpected argument")
	}
	if sinkIdx >= 0 && m < nonSinkParamCount {
		// Not enough args to fill required non-sink params.
		// The detailed "missing argument" error is raised by Apply when mapping has -2 slots.
	}

	mapping := slices.Repeat([]int{-2}, len(n.Positional))
	if sinkIdx >= 0 {
		mapping[sinkIdx] = -3 // sentinel: handled by Apply
	}

	// Post-sink params consume from the back of args.
	postSinkArgStart := m - len(postSink)
	if postSinkArgStart < 0 {
		postSinkArgStart = m
	}
	for i, p := range postSink {
		argIdx := postSinkArgStart + i
		if argIdx >= m {
			if p.Default != nil {
				mapping[postSinkSlots[i]] = -1
			}
			continue
		}
		if !p.Type.Contains(merged.Positional[argIdx].Type()) {
			err := ArgErrorPosf(argIdx, "expected %s, found %s", p.Type, merged.Positional[argIdx].Type())
			hintDecimal(err, merged.Positional[argIdx].Type(), p.Type)
			return nil, nil, err
		}
		mapping[postSinkSlots[i]] = argIdx
	}

	// Pre-sink params consume from the front, using the existing type-matching
	// algorithm. Available args for pre-sink are [0, postSinkArgStart).
	preSinkArgEnd := postSinkArgStart
	if sinkIdx < 0 {
		preSinkArgEnd = m
	}
	preSinkArgCount := min(preSinkArgEnd, len(preSink))

	lo := 0
	reqRemaining := 0
	for _, p := range preSink {
		if p.Default == nil {
			reqRemaining++
		}
	}

	for i := range preSinkArgCount {
		arg := merged.Positional[i]
		hi := len(preSink) - preSinkArgCount + i
		argsLeft := preSinkArgCount - i
		matched := false
		for slot := lo; slot <= hi; slot++ {
			if preSink[slot].Default != nil && argsLeft <= reqRemaining {
				continue
			}
			if preSink[slot].Type.Contains(arg.Type()) {
				for j := lo; j < slot; j++ {
					mapping[preSinkSlots[j]] = -1
				}
				mapping[preSinkSlots[slot]] = i
				if preSink[slot].Default == nil {
					reqRemaining--
				}
				lo = slot + 1
				matched = true
				break
			}
		}
		if !matched {
			err := ArgErrorPosf(i, "expected %s, found %s", preSink[lo].Type, arg.Type())
			hintDecimal(err, arg.Type(), preSink[lo].Type)
			return nil, nil, err
		}
	}
	for i := lo; i < len(preSink); i++ {
		if preSink[i].Default != nil {
			mapping[preSinkSlots[i]] = -1
		}
	}

	// Validate named args.
	for name := range merged.Named {
		if _, ok := n.Named[name]; !ok {
			if sinkIdx < 0 {
				return nil, nil, ArgErrorNamedPairf(name, "unexpected argument: %s", name)
			}
			// When a sink exists, unknown named args go to the sink.
			continue
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

	pos := make([]Value, len(mapping))
	for i, argIdx := range mapping {
		switch argIdx {
		case -3:
			// Sink slot — filled below.
		case -2:
			return nil, fmt.Errorf("missing argument: %s", n.Positional[i].Name)
		case -1:
			pos[i] = n.Positional[i].Default
		default:
			pos[i] = merged.Positional[argIdx]
		}
	}

	namedArgs := merged.Named

	// Pack the sink: collect middle positional args + unmatched named args.
	if n.Sink != nil {
		sinkIdx := *n.Sink

		// Determine which positional args were consumed by non-sink params.
		consumed := make([]bool, len(merged.Positional))
		for _, argIdx := range mapping {
			if argIdx >= 0 {
				consumed[argIdx] = true
			}
		}
		var sinkPos []Value
		for i, v := range merged.Positional {
			if !consumed[i] {
				sinkPos = append(sinkPos, v)
			}
		}

		// Unmatched named args go to the sink.
		var sinkNamed NamedArgs
		matchedNamed := make(NamedArgs)
		for k, v := range merged.Named {
			if _, ok := n.Named[k]; ok {
				matchedNamed[k] = v
			} else {
				if sinkNamed == nil {
					sinkNamed = make(NamedArgs)
				}
				sinkNamed[k] = v
			}
		}
		namedArgs = matchedNamed

		pos[sinkIdx] = &Arguments{Positional: sinkPos, Named: sinkNamed}
	}

	return n.F(call, pos, NamedArgsWithDefaults{
		Args:     namedArgs,
		Defaults: n.Named,
	})
}

func (*Function) aValue()          {}
func (*Function) Type() types.Type { return types.Function }

// hintDecimal adds a helpful hint when a decimal is passed where float/angle/int
// is expected but decimal is not.
func hintDecimal(err *FunctionCallError, got types.Type, expected types.Set) {
	if got != types.Decimal || expected.Contains(types.Decimal) {
		return
	}
	if expected.Contains(types.Float) || expected.Contains(types.Angle) {
		err.Hint("if loss of precision is acceptable, explicitly cast the decimal to a float with `float(value)`")
	}
}
