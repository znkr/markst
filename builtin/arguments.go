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
	"fmt"
	"slices"

	"znkr.io/markst/name"
	"znkr.io/markst/types"
	"znkr.io/markst/value"
)

var (
	Arguments = &value.Function{
		Name: "arguments",
		Positional: []value.Param{
			{Name: "args", Type: types.SetOf(types.Arguments)},
		},
		Sink: new(0),
		F:    argumentsImpl,
	}

	ArgumentsLen = &value.Function{
		Name: "arguments.len",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Arguments)},
		},
		F: argumentsLenImpl,
	}

	ArgumentsAt = &value.Function{
		Name: "arguments.at",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Arguments)},
			{Name: "key", Type: types.SetOf(types.Int, types.Str)},
		},
		F: argumentsAtImpl,
	}

	ArgumentsPos = &value.Function{
		Name: "arguments.pos",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Arguments)},
		},
		F: argumentsPosImpl,
	}

	ArgumentsNamed = &value.Function{
		Name: "arguments.named",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Arguments)},
		},
		F: argumentsNamedImpl,
	}

	ArgumentsFilter = &value.Function{
		Name: "arguments.filter",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Arguments)},
			{Name: "test", Type: types.SetOf(types.Function)},
		},
		F: argumentsFilterImpl,
	}

	ArgumentsMap = &value.Function{
		Name: "arguments.map",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Arguments)},
			{Name: "mapper", Type: types.SetOf(types.Function)},
		},
		F: argumentsMapImpl,
	}
)

func argumentsImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	return args[0].(*value.Arguments), nil
}

func argumentsLenImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	v := args[0].(*value.Arguments)
	return value.Int(len(v.Positional) + v.Named.Len()), nil
}

func argumentsAtImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	v := args[0].(*value.Arguments)
	switch key := args[1].(type) {
	case value.Int:
		if key < 0 || key >= value.Int(len(v.Positional)) {
			return nil, value.ArgErrorPosf(1, "arguments do not contain key %d and no default value was specified", key)
		}
		return v.Positional[key], nil
	case value.Str:
		val, ok := v.Named.Get(name.Make(string(key)))
		if !ok {
			return nil, value.ArgErrorPosf(1, "arguments do not contain key %q and no default value was specified", key)
		}
		return val, nil
	default:
		panic("unexpected type: " + key.Type().String())
	}
}

func argumentsPosImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	v := args[0].(*value.Arguments)
	return &value.Array{Elems: slices.Clone(v.Positional)}, nil
}

func argumentsNamedImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	v := args[0].(*value.Arguments)
	dict := new(value.Dict)
	for k, val := range v.Named.All() {
		dict.Elems.Put(value.Str(k.String()), val)
	}
	return dict, nil
}

func argumentsFilterImpl(call *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	v := args[0].(*value.Arguments)
	test := args[1].(*value.Function)

	var filteredPos []value.Value
	for _, elem := range v.Positional {
		include, poison, err := applyPredicate(call, test, elem)
		if err != nil {
			return nil, fmt.Errorf("error calling test function: %w", err)
		}
		if poison != nil {
			return poison, nil
		}
		if include {
			filteredPos = append(filteredPos, elem)
		}
	}

	var filteredNamed value.NamedArgs
	for k, elem := range v.Named.All() {
		include, poison, err := applyPredicate(call, test, elem)
		if err != nil {
			return nil, fmt.Errorf("error calling test function: %w", err)
		}
		if poison != nil {
			return poison, nil
		}
		if include {
			filteredNamed.Put(k, elem)
		}
	}

	return &value.Arguments{Positional: filteredPos, Named: filteredNamed}, nil
}

func argumentsMapImpl(call *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	v := args[0].(*value.Arguments)
	mapper := args[1].(*value.Function)

	mappedPos := make([]value.Value, len(v.Positional))
	for i, elem := range v.Positional {
		res, poison, err := applyMapper(call, mapper, elem)
		if err != nil {
			return nil, fmt.Errorf("error calling mapper function: %w", err)
		}
		if poison != nil {
			return poison, nil
		}
		mappedPos[i] = res
	}

	var mappedNamed value.NamedArgs
	for k, elem := range v.Named.All() {
		res, poison, err := applyMapper(call, mapper, elem)
		if err != nil {
			return nil, fmt.Errorf("error calling mapper function: %w", err)
		}
		if poison != nil {
			return poison, nil
		}
		mappedNamed.Put(k, res)
	}

	return &value.Arguments{Positional: mappedPos, Named: mappedNamed}, nil
}
