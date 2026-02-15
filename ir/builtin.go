package ir

import (
	"fmt"
	"strconv"

	"znkr.io/writst/ir/types"
)

var (
	builtinRepr = &Function{
		Name:       "repr",
		Positional: []types.Set{types.Any},
		F:          builtinReprImpl,
	}

	builtinType = &Function{
		Name:       "type",
		Positional: []types.Set{types.Any},
		// F is set in init() to avoid an initialization cycle.
	}
)

func init() {
	builtinType.F = builtinTypeImpl
}

func builtinReprImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	switch v := args[0].(type) {
	case None:
		return Str("none"), nil
	case Bool:
		if v {
			return Str("true"), nil
		}
		return Str("false"), nil
	case Int:
		return Str(strconv.FormatInt(int64(v), 10)), nil
	case Float:
		return Str(strconv.FormatFloat(float64(v), 'g', -1, 64)), nil
	default:
		panic(fmt.Sprintf("repr() not implemented for type %s", args[0].Type()))
	}
}

func builtinTypeImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	return reflectedTypes[args[0].Type()], nil
}
