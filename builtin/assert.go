package builtin

import (
	"fmt"

	"znkr.io/markst/internal/names"
	"znkr.io/markst/name"
	"znkr.io/markst/types"
	"znkr.io/markst/value"
)

// Assert is the global `assert` function. It is callable directly to check a
// boolean condition and exposes `assert.eq` and `assert.ne` through its scope.
var Assert = &value.Function{
	Name: "assert",
	Positional: []value.Param{
		{Name: "condition", Type: types.SetOf(types.Bool)},
	},
	Named: value.NamedParams{
		names.Message: value.Param{Name: "message", Type: types.SetOf(types.Str), Default: value.None{}},
	},
	F: assertImpl,
	Scope: map[name.Name]value.Value{
		names.Eq: AssertEq,
		names.Ne: AssertNe,
	},
}

var AssertEq = &value.Function{
	Name: "assert.eq",
	Positional: []value.Param{
		{Name: "left", Type: types.Any},
		{Name: "right", Type: types.Any},
	},
	Named: value.NamedParams{
		names.Message: value.Param{Name: "message", Type: types.SetOf(types.Str), Default: value.None{}},
	},
	F: assertEqImpl,
}

var AssertNe = &value.Function{
	Name: "assert.ne",
	Positional: []value.Param{
		{Name: "left", Type: types.Any},
		{Name: "right", Type: types.Any},
	},
	Named: value.NamedParams{
		names.Message: value.Param{Name: "message", Type: types.SetOf(types.Str), Default: value.None{}},
	},
	F: assertNeImpl,
}

func assertImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	if bool(args[0].(value.Bool)) {
		return value.None{}, nil
	}
	if msg, ok := assertMessage(named); ok {
		return nil, fmt.Errorf("assertion failed: %s", msg)
	}
	return nil, fmt.Errorf("assertion failed")
}

func assertEqImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	if value.Equal(args[0], args[1]) {
		return value.None{}, nil
	}
	if msg, ok := assertMessage(named); ok {
		return nil, fmt.Errorf("equality assertion failed: %s", msg)
	}
	return nil, fmt.Errorf("equality assertion failed: value %s was not equal to %s", repr(args[0]), repr(args[1]))
}

func assertNeImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	if !value.Equal(args[0], args[1]) {
		return value.None{}, nil
	}
	if msg, ok := assertMessage(named); ok {
		return nil, fmt.Errorf("inequality assertion failed: %s", msg)
	}
	return nil, fmt.Errorf("inequality assertion failed: value %s was equal to %s", repr(args[0]), repr(args[1]))
}

// assertMessage returns the explicit `message` argument, if one was provided.
func assertMessage(named value.NamedArgsWithDefaults) (string, bool) {
	if !named.IsSet(names.Message) {
		return "", false
	}
	return string(named.Get(names.Message).(value.Str)), true
}

// repr renders a value the way the `repr` built-in does, for diagnostics.
func repr(v value.Value) string {
	r, err := reprImpl(nil, []value.Value{v}, value.NamedArgsWithDefaults{})
	if err != nil {
		return v.Type().String()
	}
	return string(r.(value.Str))
}
