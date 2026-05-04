package builtin

import (
	"unique"

	"znkr.io/writst/types"
	"znkr.io/writst/value"
)

var (
	Arguments = &value.Function{
		Name: "arguments",
		Bind: func(_ *value.Function, args *value.Arguments) (*value.Arguments, []int, error) {
			return args, nil, nil
		},
		F: argumentsImpl,
	}

	ArgumentsLen = &value.Function{
		Name: "arguments.len",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Arguments)},
		},
		F: argumentsLenImpl,
	}

	ArgumentsAt = &value.Function{
		Name: "arguments.at",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Arguments)},
			{Name: "key", Type: types.SetOf(types.Int, types.Str)},
		},
		F: argumentsAtImpl,
	}
)

func argumentsImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	return &value.Arguments{Positional: args, Named: named.Args}, nil
}

func argumentsLenImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	v := args[0].(*value.Arguments)
	return value.Int(len(v.Positional) + len(v.Named)), nil
}

func argumentsAtImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	v := args[0].(*value.Arguments)
	switch key := args[1].(type) {
	case value.Int:
		if key < 0 || key >= value.Int(len(v.Positional)) {
			return nil, value.ArgErrorPosf(1, "arguments do not contain key %d and no default value was specified", key)
		}
		return v.Positional[key], nil
	case value.Str:
		val, ok := v.Named[unique.Make(string(key))]
		if !ok {
			return nil, value.ArgErrorPosf(1, "arguments do not contain key %q and no default value was specified", key)
		}
		return val, nil
	default:
		panic("unexpected type: " + key.Type().String())
	}
}
