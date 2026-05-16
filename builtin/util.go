package builtin

import (
	"fmt"

	"znkr.io/writst/value"
)

// applyPredicate calls fn with v as its sole positional argument and returns
// the resulting boolean. It is used by filter-like builtins.
//
// If the predicate body errored, the returned value is a [*value.Error]
// (already recorded on the session). Treat that as "skip this element" so
// the user sees the predicate's original error and not a redundant
// "predicate must return a boolean" diagnostic.
func applyPredicate(fn *value.Function, v value.Value) (bool, error) {
	fcc := &value.FunctionCallContext{} // TODO: no span for the call context!
	res, err := fn.Apply(fcc, &value.Arguments{Positional: []value.Value{v}})
	if err != nil {
		return false, err
	}
	if _, ok := value.IsError(res); ok {
		return false, nil
	}
	include, ok := res.(value.Bool)
	if !ok {
		return false, fmt.Errorf("test function must return a boolean")
	}
	return bool(include), nil
}

// applyMapper calls fn with v as its sole positional argument and returns the
// result. It is used by map-like builtins.
func applyMapper(fn *value.Function, v value.Value) (value.Value, error) {
	fcc := &value.FunctionCallContext{} // TODO: no span for the call context!
	return fn.Apply(fcc, &value.Arguments{Positional: []value.Value{v}})
}
