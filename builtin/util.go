package builtin

import (
	"fmt"

	"znkr.io/writst/value"
)

// applyCallback calls fn with args and separates the two ways a user callback
// can fail.
//
// poison is a [*value.Error] the callback evaluated to: the diagnostic is
// already on the session, and the value is the evaluator's poison marker. A
// builtin that gets one must return it as its own result rather than carry on
// with a plausible-looking value — an Error operand travels through the SSA
// value table without being re-recorded, which is what keeps a failed
// predicate from also being reported as, say, a non-content value at the end
// of the document. Continuing instead hands downstream code something that
// looks fine and earns a second, misleading diagnostic.
//
// err is the other kind: a failure that is not yet recorded anywhere, to be
// wrapped and returned by the builtin in the usual way.
func applyCallback(fn *value.Function, args ...value.Value) (res value.Value, poison *value.Error, err error) {
	fcc := &value.FunctionCallContext{} // TODO: no span for the call context!
	res, err = fn.Apply(fcc, &value.Arguments{Positional: args})
	if err != nil {
		return nil, nil, err
	}
	if e, ok := value.IsError(res); ok {
		return nil, e, nil
	}
	return res, nil, nil
}

// applyPredicate calls fn with v as its sole positional argument and returns
// the resulting boolean. It is used by filter-like builtins. See
// [applyCallback] for poison.
func applyPredicate(fn *value.Function, v value.Value) (include bool, poison *value.Error, err error) {
	res, poison, err := applyCallback(fn, v)
	if err != nil || poison != nil {
		return false, poison, err
	}
	b, ok := res.(value.Bool)
	if !ok {
		return false, nil, fmt.Errorf("test function must return a boolean")
	}
	return bool(b), nil, nil
}

// applyMapper calls fn with v as its sole positional argument and returns the
// result. It is used by map-like builtins. See [applyCallback] for poison.
func applyMapper(fn *value.Function, v value.Value) (res value.Value, poison *value.Error, err error) {
	return applyCallback(fn, v)
}
