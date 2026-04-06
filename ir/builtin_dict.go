package ir

import (
	"znkr.io/writst/ir/types"
)

var (
	builtinDictAt = &Function{
		Name: "dictionary.at",
		Positional: []Param{
			{Name: "self", Type: types.SetOf(types.Dict)},
			{Name: "key", Type: types.SetOf(types.Str)},
		},
		F: builtinDictAtImpl,
	}
)

func builtinDictAtImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	dict := args[0].(Dict)
	key := args[1].(Str)
	return dict[key], nil
}
