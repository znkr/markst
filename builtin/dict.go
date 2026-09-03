// Copyright 2026 Florian Zenker (flo@znkr.io)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package builtin

import (
	"znkr.io/markst/internal/names"
	"znkr.io/markst/types"
	"znkr.io/markst/value"
	value0 "znkr.io/markst/value"
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
		F:        dictAtImpl,
		Accessor: true,
	}

	DictInsert = &value.Function{
		Name: "dictionary.insert",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Dict)},
			{Name: "key", Type: types.SetOf(types.Str)},
			{Name: "value", Type: types.Any},
		},
		Impure: true,
		F:      dictInsertImpl,
	}

	DictRemove = &value.Function{
		Name: "dictionary.remove",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Dict)},
			{Name: "key", Type: types.SetOf(types.Str)},
		},
		Named: value.NamedParams{
			names.Default: value.Param{Name: "default", Type: types.Any},
		},
		Impure: true,
		F:      dictRemoveImpl,
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

func dictInsertImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	dict := args[0].(*value.Dict)
	key := args[1].(value.Str)
	dict.Elems.Put(key, args[2])
	return value.None{}, nil
}

func dictRemoveImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	dict := args[0].(*value.Dict)
	key := args[1].(value.Str)
	rem, ok := dict.Elems.Delete(key)
	if !ok {
		if def, hasDef := named.Lookup(names.Default); hasDef {
			return def, nil
		}
		return nil, value.ArgErrorPosf(1, "dictionary does not contain key %q and no default value was specified", key)
	}
	return rem, nil
}

func dictAtImpl(fc *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	dict := args[0].(*value.Dict)
	key := args[1].(value.Str)
	val, ok := dict.Elems.Get(key)
	if !ok {
		if def, hasDef := named.Lookup(names.Default); hasDef {
			return def, nil
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
