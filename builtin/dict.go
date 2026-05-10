package builtin

import (
	"znkr.io/writst/internal/names"
	"znkr.io/writst/types"
	"znkr.io/writst/value"
	value0 "znkr.io/writst/value"
)

var (
	DictPairs = &value.Function{
		Name: "dictionary.pairs",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Dict)},
		},
		F: dictPairsImpl,
	}

	DictAt = &value.Function{
		Name: "dictionary.at",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Dict)},
			{Name: "key", Type: types.SetOf(types.Str)},
		},
		Named: value.NamedParams{
			names.Default: value.Param{Name: "default", Type: types.Any},
		},
		F: dictAtImpl,
	}
)

func dictPairsImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	dict := args[0].(*value.Dict)
	pairs := make([]value.Value, 0, dict.Elems.Len())
	for k, v := range dict.Elems.All() {
		pairs = append(pairs, &value.Array{Elems: []value.Value{k, v}})
	}
	return &value.Array{Elems: pairs}, nil
}

func dictAtImpl(fc *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	dict := args[0].(*value.Dict)
	key := args[1].(value.Str)
	val, ok := dict.Elems.Get(key)
	if !ok {
		if named.IsSet(names.Default) {
			return named.Get(names.Default), nil
		}
		return nil, value.ArgErrorPosf(1, "dictionary does not contain key %q and no default value was specified", key)
	}
	if fc.Setter != nil {
		*fc.Setter = func(v value0.Value) {
			dict.Elems.Put(key, v)
		}
	}
	return val, nil
}
