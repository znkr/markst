package value

import (
	"fmt"
	"slices"
	"time"

	"znkr.io/markst/name"
	"znkr.io/markst/syntax"
	"znkr.io/markst/types"
)

// Function is a Markst function: a closure written in a document, or a
// builtin. It holds the parameter signature and the implementation.
type Function struct {
	// Name is the function's name.
	Name string

	// Positional describes the function's positional parameters in order.
	// If Sink is set, the parameter at index *Sink is the sink parameter
	// and receives a *Arguments value containing surplus positional args
	// and unmatched named args.
	Positional []Param

	// Sink, if set, marks Positional[*Sink] as the sink parameter. Parameters
	// before it take arguments from the front, parameters after it from the
	// back, and everything remaining, together with any unmatched named
	// arguments, is collected into an *Arguments passed at that index. A
	// function that should reject stray named arguments checks the sink's
	// Named itself; math.vec, math.mat, math.cases and array.zip do.
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

	// Validate, if set, runs on the merged named arguments after the generic
	// bind checks pass. It checks what the type system cannot express, such as
	// a `delim` that must be a single valid delimiter character, and returns
	// an error located at the offending argument. It runs for both direct
	// calls and `#set` rules, since both bind through [bind].
	Validate func(named NamedArgs) *FunctionCallError

	// Impure marks a function with observable effects beyond its return
	// value, such as mutating an operand or changing session state. The zero
	// value, false, means the function is pure.
	Impure bool

	// Accessor marks a method whose result is a mutable place into its
	// receiver rather than a fresh value, meaning it sets a
	// [FunctionCallContext.Setter]. `at`, `first` and `last` on an array or
	// dict are the cases. A mutating method may be chained onto an accessor's
	// result, as in `m.at(1).at(0).push(5)`, without reporting "cannot mutate
	// a temporary value". The place check reads this flag on the resolved
	// method rather than comparing method names.
	Accessor bool

	// Scope holds the functions reachable by field access on the function
	// value itself, such as assert.eq. Only builtins have one; it is nil for
	// plain functions and user-defined closures.
	Scope map[name.Name]Value

	// Closure reports whether this value is a user-defined closure (as opposed
	// to a built-in function). Field access on closures is always an error.
	Closure bool
}

// NamedParams is a function's named parameters, by name.
type NamedParams map[name.Name]Param

// Param is one function parameter: its name, the types it accepts, and its
// default. A nil default means the parameter is required.
type Param struct {
	Name    string
	Type    types.Set
	Default Value
}

// NamedArgsWithDefaults is what a call passed by name, together with the
// defaults for what it did not pass. An implementation reads it with
// [NamedArgsWithDefaults.Get].
type NamedArgsWithDefaults struct {
	Args     NamedArgs
	Defaults NamedParams
}

// Get returns the named argument the call passed, or the parameter's default
// if it passed none, or [None] if there is no default either.
func (n NamedArgsWithDefaults) Get(name name.Name) Value {
	v, _ := n.Lookup(name)
	return v
}

// Lookup returns what [NamedArgsWithDefaults.Get] would, and reports whether
// there was a value at all — false only when the call passed nothing and the
// parameter has no default, in which case the value is [None].
//
// It does not report whether the call itself passed the argument: a parameter
// left at its default reports true. Read Args for that.
func (n NamedArgsWithDefaults) Lookup(name name.Name) (Value, bool) {
	if v, ok := n.Args.Get(name); ok {
		return v, true
	}
	if def, ok := n.Defaults[name]; ok && def.Default != nil {
		return def.Default, true
	}
	return None{}, false
}

// With returns a copy of the function with args already supplied, so calling
// the copy passes them ahead of whatever the call site passes. It is how a
// method call binds its receiver as the first argument.
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
	// With too few arguments to fill the required non-sink params, Apply
	// reports "missing argument" for each slot still at -2.

	mapping := slices.Repeat([]int{-2}, len(n.Positional))
	if sinkIdx >= 0 {
		mapping[sinkIdx] = -3 // sentinel: handled by Apply
	}

	// Positional arguments are consumed front to back: parameters before the
	// sink first, then the sink takes everything beyond the non-sink
	// parameters, then the parameters after it. With too few arguments, the
	// trailing post-sink parameters are the ones left unfilled.
	sinkSize := 0
	if sinkIdx >= 0 {
		sinkSize = max(0, m-nonSinkParamCount)
	}

	// Pre-sink params consume from the front, matching on type.
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
					// Only an optional parameter takes its default when nothing
					// matches it. A required one stays -2, so Apply reports the
					// missing argument rather than passing the builtin a nil.
					if preSink[j].Default != nil {
						mapping[preSinkSlots[j]] = -1
					}
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
			// With a sink present, unknown named arguments go into it. A function
			// that should not accept them rejects them from the sink itself.
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

// FunctionCallContext is what the evaluator hands an implementation about the
// call it is running: where it was written, and the hooks it may use.
type FunctionCallContext struct {
	Span syntax.Span

	// Now is the instant the document is being rendered at, which
	// `datetime.today` reads the current date from. It is passed through the
	// call context rather than read from the clock so that a render is
	// reproducible: two evaluations of the same source with the same Now
	// produce the same document. A zero value means read the clock.
	Now time.Time

	// Setter is how a function makes its result assignable, as `array.at()`
	// is. A function that returns a place into its receiver sets this, and the
	// evaluator calls it to perform the write.
	Setter *func(Value)

	// Runtime is the evaluator state this call runs under. The evaluator sets
	// it; nothing else interprets it. A closure needs it because it outlives
	// the evaluation that created it: a function defined in a library and
	// called while compiling a document must record its diagnostics, labels
	// and document properties on the document being compiled, not on the
	// library's finished evaluation.
	//
	// Builtins do not use it but must pass it on. A builtin that invokes a
	// user callback forwards the whole context (see the builtin package's
	// applyCallback) so the callback runs in the same runtime as its caller.
	Runtime any

	// Refs is where a builtin that builds a [Ref] reports which label the
	// document must define. A reference cannot be resolved where it is built,
	// since the label it names may be attached further down, so the
	// evaluation collects them and resolves them once the document is
	// complete.
	//
	// The evaluator sets it on every call it makes. It is nil for a call made
	// outside an evaluation, and for the show-rule transforms realization
	// calls directly, where a builtin cannot build a reference: a transform
	// receives content, and `ref` takes a label.
	Refs RefRecorder
}

// RefRecorder collects the references an evaluation builds, so they can be
// resolved against the document's labels once it is finished. A reference may
// name a label written anywhere, so none of them can be answered before then.
// See [FunctionCallContext.Refs].
type RefRecorder interface {
	// RecordRef records a reference to target, built at span.
	RecordRef(target name.Name, span syntax.Span)
}

// Apply calls the function with args, on top of anything [Function.With]
// already bound. It returns an error if the arguments do not fit the
// signature.
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
			// An anonymous positional parameter, an underscore or a
			// destructuring pattern, is named "_". Report it as a pattern
			// parameter.
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
