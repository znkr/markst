package value

import (
	"fmt"
	"slices"
	"time"

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
	// into a *Arguments passed at that index. A function whose sink should not
	// silently swallow stray named args inspects the sink's Named itself (see
	// e.g. math.vec/mat/cases and array.zip).
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

	// Validate, if set, runs after the generic bind checks succeed, receiving
	// the merged named arguments. It performs value-level validation the coarse
	// type system can't express (e.g. a `delim` that must be a single valid
	// delimiter character) and returns a spanned error on failure. It runs for
	// both direct calls and `#set` rules, since both bind through [bind].
	Validate func(named NamedArgs) *FunctionCallError

	// Impure marks the function as having observable side effects beyond its
	// return value (mutating an operand, touching session-global state, etc.).
	// The default — false — means the function is pure.
	Impure bool

	// Accessor marks a method whose result is a mutable place into its receiver
	// rather than a fresh value — i.e. it registers a
	// [FunctionCallContext.Setter] (array/dict `at`, `first`, `last`). A mutating
	// method may be chained onto an accessor's result (`m.at(1).at(0).push(5)`)
	// without hitting "cannot mutate a temporary value"; the method-call
	// place check consults this flag on the resolved method rather than matching
	// method names.
	Accessor bool

	// Scope holds associated functions reachable via field access on the
	// function value itself (e.g. assert.eq). Only built-in functions populate
	// it; it is nil for plain functions and user-defined closures.
	Scope map[name.Name]Value

	// Closure reports whether this value is a user-defined closure (as opposed
	// to a built-in function). Field access on closures is always an error.
	Closure bool
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
	_, ok := n.Args.Get(name)
	return ok
}

// Get returns the value of a named argument, falling back to its default
// value if not explicitly provided, or [None] if there is no default.
func (n *NamedArgsWithDefaults) Get(name name.Name) Value {
	v, ok := n.Args.Get(name)
	if !ok {
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
		Impure:     n.Impure,
		Accessor:   n.Accessor,
		Scope:      n.Scope,
		Closure:    n.Closure,
	}, nil
}

// callableAsFunction checks whether v satisfies a parameter of type pt,
// coercing a type value to its constructor when the parameter expects a
// function. A type is callable — invoking it runs its constructor — so passing
// a callable type where a function is required (e.g. `array.map(str)`) binds the
// type's constructor function, matching Typst. Returns the possibly-coerced
// value and whether it satisfies pt.
func callableAsFunction(pt types.Set, v Value) (Value, bool) {
	if pt.Contains(v.Type()) {
		return v, true
	}
	if pt.Contains(types.Function) {
		if t, ok := v.(*Type); ok && t.Constructor != nil {
			return t.Constructor, true
		}
	}
	return v, false
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
	// When there are too few args to fill the required non-sink params, the
	// detailed "missing argument" error is raised by Apply for the unfilled
	// (-2) slots.

	mapping := slices.Repeat([]int{-2}, len(n.Positional))
	if sinkIdx >= 0 {
		mapping[sinkIdx] = -3 // sentinel: handled by Apply
	}

	// Positional args are consumed strictly front-to-back: pre-sink params
	// first, then the sink absorbs the surplus (args beyond all non-sink
	// params), then post-sink params. When there are too few args, the
	// trailing post-sink params are the ones left unfilled.
	sinkSize := 0
	if sinkIdx >= 0 {
		sinkSize = max(0, m-nonSinkParamCount)
	}

	// Pre-sink params consume from the front, using the existing type-matching
	// algorithm.
	preSinkArgCount := min(m, len(preSink))

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
			if coerced, ok := callableAsFunction(preSink[slot].Type, arg); ok {
				merged.Positional[i] = coerced
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

	// Post-sink params consume from the front, continuing after the args taken
	// by the pre-sink params and the sink.
	postSinkArgStart := preSinkArgCount + sinkSize
	for i, p := range postSink {
		argIdx := postSinkArgStart + i
		if argIdx >= m {
			if p.Default != nil {
				mapping[postSinkSlots[i]] = -1
			}
			continue
		}
		if coerced, ok := callableAsFunction(p.Type, merged.Positional[argIdx]); ok {
			merged.Positional[argIdx] = coerced
		} else {
			err := ArgErrorPosf(argIdx, "expected %s, found %s", p.Type, merged.Positional[argIdx].Type())
			hintDecimal(err, merged.Positional[argIdx].Type(), p.Type)
			return nil, nil, err
		}
		mapping[postSinkSlots[i]] = argIdx
	}

	// Validate named args. Callable-type coercions are collected and applied
	// after the loop so the map isn't mutated mid-iteration.
	type namedCoercion struct {
		key name.Name
		val Value
	}
	var namedCoercions []namedCoercion
	for name, val := range merged.Named.All() {
		if _, ok := n.Named[name]; !ok {
			if sinkIdx < 0 {
				for _, p := range n.Positional {
					if p.Name == name.String() {
						err := ArgErrorNamedf(name, "the argument `%s` is positional", name)
						err.Hint(fmt.Sprintf("try removing `%s:`", name))
						return nil, nil, err
					}
				}
				return nil, nil, ArgErrorNamedPairf(name, "unexpected argument: %s", name)
			}
			// When a sink exists, unknown named args go to the sink; a function
			// that doesn't want to accept them rejects them from the sink itself.
			continue
		}
		coerced, ok := callableAsFunction(n.Named[name].Type, val)
		if !ok {
			return nil, nil, ArgErrorNamedf(name, "expected %s, found %s", n.Named[name].Type, val.Type())
		}
		if coerced != val {
			namedCoercions = append(namedCoercions, namedCoercion{name, coerced})
		}
	}
	for _, c := range namedCoercions {
		merged.Named.Put(c.key, c.val)
	}
	if n.Validate != nil {
		if err := n.Validate(merged.Named); err != nil {
			return nil, nil, err
		}
	}
	return merged, mapping, nil
}

// FunctionCallContext carries call-site information passed to function
// implementations.
type FunctionCallContext struct {
	Span syntax.Span

	// Now is the instant the document is being rendered at, which
	// `datetime.today` reads the current date off. It is threaded through the
	// call context rather than read from the clock so that a render is
	// reproducible: two evaluations of the same source with the same Now
	// produce the same document. A zero value means "read the clock".
	Now time.Time

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
			// An anonymous positional param (underscore or destructuring
			// pattern) is named "_"; report it as a pattern parameter.
			pname := n.Positional[i].Name
			if pname == "_" {
				pname = "pattern parameter"
			}
			return nil, fmt.Errorf("missing argument: %s", pname)
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
		var sinkNamed, matchedNamed NamedArgs
		for k, v := range merged.Named.All() {
			if _, ok := n.Named[k]; ok {
				matchedNamed.Put(k, v)
			} else {
				sinkNamed.Put(k, v)
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
