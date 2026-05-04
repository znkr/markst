package builtin

import (
	"znkr.io/writst/types"
	"znkr.io/writst/value"
	value0 "znkr.io/writst/value"
)

var (
	DictAt = &value.Function{
		Name: "dictionary.at",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Dict)},
			{Name: "key", Type: types.SetOf(types.Str)},
		},
		F: dictAtImpl,
	}
)

func dictAtImpl(fc *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	dict := args[0].(*value.Dict)
	key := args[1].(value.Str)
	val, ok := dict.Elems.Get(key)
	if !ok {
		return value.None{}, nil
	}
	if fc.Setter != nil {
		*fc.Setter = func(v value0.Value) {
			dict.Elems.Put(key, v)
		}
	}
	return val, nil
}
