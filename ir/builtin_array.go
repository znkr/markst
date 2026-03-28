package ir

import (
	"znkr.io/writst/ir/internal/names"
	"znkr.io/writst/ir/types"
	"znkr.io/writst/syntax"
)

var (
	builtinArray = &Function{
		Name:       "array",
		Positional: []types.Set{types.Any},
		F:          builtinArrayImpl,
	}

	builtinRange = &Function{
		Name:       "range",
		Positional: []types.Set{types.SetOf(types.Int), types.SetOf(types.Int)},
		Named: NamedParams{
			names.Step: NamedParam{Type: types.SetOf(types.Int), Default: Int(1)},
		},
		F: builtinRangeImpl,
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

	builtinArraySum = &Function{
		Name:       "array.sum",
		Positional: []types.Set{types.SetOf(types.Array)},
		F:          builtinArraySumImpl,
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

func builtinRangeImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	start, end := args[0].(Int), args[1].(Int)
	step := named.Get(names.Step).(Int)

	var result Array
	for i := start; i < end; i += step {
		result = append(result, Int(i))
	}
	return result, nil
}

func builtinArrayJoinImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	arr := args[0].(Array)
	v, err := join(arr)
	if err != nil {
		return nil, ArgErrorPosf(0, "%s", err.Error())
	}
	return v, nil
}

func builtinArraySumImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	arr := args[0].(Array)
	if len(arr) == 0 {
		return none, nil
	}
	sum := arr[0]
	for _, v := range arr[1:] {
		if sum.Type() != v.Type() {
			return nil, ArgErrorPosf(0, "can only sum arrays that have elements of the same type, got %s and %s", sum.Type(), v.Type())
		}
	}
	op := binops[binopKey{syntax.Add, sum.Type(), sum.Type()}]
	if op == nil {
		return nil, ArgErrorPosf(0, "unsupported type for array.sum: %s", sum.Type())
	}
	for _, v := range arr[1:] {
		sum = op(sum, v)
	}
	return sum, nil
}
