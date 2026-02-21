package ir

import (
	"unique"

	"znkr.io/writst/ir/types"
)

var (
	builtinLabel = &Function{
		Name:       "label",
		Positional: []types.Set{types.SetOf(types.Str)},
		F:          builtinLabelImpl,
	}
)

func builtinLabelImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	switch v := args[0].(type) {
	case Str:
		if v == "" {
			return nil, ArgErrorPosf(0, "label name must not be empty")
		}
		return &Label{Name: unique.Make(string(v))}, nil
	default:
		panic("unexpected type: " + v.Type().String())
	}
}
