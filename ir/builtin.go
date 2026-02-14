package ir

import (
	"fmt"
	"strconv"
)

var (
	builtinRepr = &Function{
		Name:          "repr",
		NumPositional: 1,
		F:             builtinReprImpl,
	}

	builtinType = &Function{
		Name:          "type",
		NumPositional: 1,
		// F is set in init() to avoid an initialization cycle.
	}
)

func init() {
	builtinType.F = builtinTypeImpl
}

func builtinReprImpl(args []Value, named NamedArgsWithDefaults) (Value, error) {
	switch v := args[0].(type) {
	case None:
		return String("none"), nil
	case Bool:
		if v {
			return String("true"), nil
		}
		return String("false"), nil
	case Int:
		return String(strconv.FormatInt(int64(v), 10)), nil
	case Float:
		return String(strconv.FormatFloat(float64(v), 'g', -1, 64)), nil
	default:
		panic(fmt.Sprintf("repr() not implemented for type %s", args[0].Type()))
	}
}

func builtinTypeImpl(args []Value, named NamedArgsWithDefaults) (Value, error) {
	return reflectedTypes[args[0].Type()], nil
}
