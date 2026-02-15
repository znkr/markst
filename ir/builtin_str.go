package ir

import (
	"strconv"

	"znkr.io/writst/ir/types"
)

var (
	builtinStr = &Function{
		Name:       "str",
		Positional: []types.Set{types.Any},
		F:          builtinStrImpl,
	}
)

func builtinStrImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	switch v := args[0].(type) {
	case Bytes:
		return Str(v), nil
	case Int:
		r := strconv.FormatInt(int64(v), 10)
		if r[0] == '-' {
			r = "\u2212" + r[1:]
		}
		return Str(r), nil
	case Float:
		r := strconv.FormatFloat(float64(v), 'f', -1, 64)
		if r[0] == '-' {
			r = "\u2212" + r[1:]
		}
		return Str(r), nil
	case *Label:
		return Str(v.Name.Value()), nil
	default:
		panic("unexpected type: " + v.Type().String())
	}
}
