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

func builtinDictAtImpl(fc *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	dict := args[0].(*Dict)
	key := args[1].(Str)
	value, ok := dict.Elems.Get(key)
	if !ok {
		return none, nil
	}
	if fc.setter != nil {
		*fc.setter = func(v Value) {
			dict.Elems.Put(key, v)
		}
	}
	return value, nil
}
