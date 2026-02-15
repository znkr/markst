package ir

import (
	"znkr.io/writst/ir/internal/names"
	"znkr.io/writst/ir/types"
)

var (
	builtinArray = &Function{
		Name:       "array",
		Positional: []types.Set{types.Any},
		F:          builtinArrayImpl,
	}

	builtinArrayJoin = &Function{
		Name:       "array.join",
		Positional: []types.Set{types.SetOf(types.Array), types.Any},
		Named: NamedParams{
			names.Last:    NamedParam{Type: types.Any},
			names.Default: NamedParam{Type: types.Any},
		},
		F: builtinArrayJoinImpl,
	}

	builtinRange = &Function{
		Name:       "range",
		Positional: []types.Set{types.SetOf(types.Int), types.SetOf(types.Int)},
		Named: NamedParams{
			names.Step: NamedParam{Type: types.SetOf(types.Int), Default: Int(1)},
		},
		F: builtinRangeImpl,
	}
)

func builtinArrayImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	switch v := args[0].(type) {
	case Bytes:
		var result Array
		for _, b := range []byte(v) {
			result = append(result, Int(b))
		}
		return result, nil
	default:
		panic("unexpected type: " + v.Type().String())
	}
}

func builtinArrayJoinImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	panic("builtinJoinImpl is not implemented yet")
}

func builtinRangeImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	start, end := args[0].(Int), args[1].(Int)
	step := named.Get(names.Step).(Int)

	var result Array
	for i := start; i < end; i += step {
		result = append(result, Int(i))
	}
	return result, nil
}
