package ir

import (
	"unique"

	"znkr.io/writst/ir/types"
)

var (
	builtinArguments = &Function{
		Name: "arguments",
		Bind: func(_ *Function, args *Arguments) (*Arguments, []int, error) {
			return args, nil, nil
		},
		F: builtinArgumentsImpl,
	}

	builtinArgumentsLen = &Function{
		Name: "arguments.len",
		Positional: []Param{
			{Name: "self", Type: types.SetOf(types.Arguments)},
		},
		F: builtinArgumentsLenImpl,
	}

	builtinArgumentsAt = &Function{
		Name: "arguments.at",
		Positional: []Param{
			{Name: "self", Type: types.SetOf(types.Arguments)},
			{Name: "key", Type: types.SetOf(types.Int, types.Str)},
		},
		F: builtinArgumentsAtImpl,
	}
)

func builtinArgumentsImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	return &Arguments{Positional: args, Named: named.Args}, nil
}

func builtinArgumentsLenImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	v := args[0].(*Arguments)
	return Int(len(v.Positional) + len(v.Named)), nil
}

func builtinArgumentsAtImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	v := args[0].(*Arguments)
	switch key := args[1].(type) {
	case Int:
		if key < 0 || key >= Int(len(v.Positional)) {
			return nil, ArgErrorPosf(1, "arguments do not contain key %d and no default value was specified", key)
		}
		return v.Positional[key], nil
	case Str:
		val, ok := v.Named[unique.Make(string(key))]
		if !ok {
			return nil, ArgErrorPosf(1, "arguments do not contain key %q and no default value was specified", key)
		}
		return val, nil
	default:
		panic("unexpected type: " + key.Type().String())
	}
}
