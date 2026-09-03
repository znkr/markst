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

	"znkr.io/markst/internal/names"
	"znkr.io/markst/name"
	"znkr.io/markst/types"
	"znkr.io/markst/value"
)

// Assert is the global `assert` function. It is callable directly to check a
// boolean condition and exposes `assert.eq` and `assert.ne` through its scope.
var Assert = &value.Function{
	Name: "assert",
	Positional: []value.Param{
		{Name: "condition", Type: types.SetOf(types.Bool)},
	},
	Named: value.NamedParams{
		names.Message: value.Param{Name: "message", Type: types.SetOf(types.Str)},
	},
	F: assertImpl,
	Scope: map[name.Name]value.Value{
		names.Eq: AssertEq,
		names.Ne: AssertNe,
	},
}

var AssertEq = &value.Function{
	Name: "assert.eq",
	Positional: []value.Param{
		{Name: "left", Type: types.Any},
		{Name: "right", Type: types.Any},
	},
	Named: value.NamedParams{
		names.Message: value.Param{Name: "message", Type: types.SetOf(types.Str)},
	},
	F: assertEqImpl,
}

var AssertNe = &value.Function{
	Name: "assert.ne",
	Positional: []value.Param{
		{Name: "left", Type: types.Any},
		{Name: "right", Type: types.Any},
	},
	Named: value.NamedParams{
		names.Message: value.Param{Name: "message", Type: types.SetOf(types.Str)},
	},
	F: assertNeImpl,
}

func assertImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	if bool(args[0].(value.Bool)) {
		return value.None{}, nil
	}
	if msg, ok := assertMessage(named); ok {
		return nil, fmt.Errorf("assertion failed: %s", msg)
	}
	return nil, fmt.Errorf("assertion failed")
}

func assertEqImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	if value.Equal(args[0], args[1]) {
		return value.None{}, nil
	}
	if msg, ok := assertMessage(named); ok {
		return nil, fmt.Errorf("equality assertion failed: %s", msg)
	}
	return nil, fmt.Errorf("equality assertion failed: value %s was not equal to %s", repr(args[0]), repr(args[1]))
}

func assertNeImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	if !value.Equal(args[0], args[1]) {
		return value.None{}, nil
	}
	if msg, ok := assertMessage(named); ok {
		return nil, fmt.Errorf("inequality assertion failed: %s", msg)
	}
	return nil, fmt.Errorf("inequality assertion failed: value %s was equal to %s", repr(args[0]), repr(args[1]))
}

// assertMessage returns the explicit `message` argument, if one was provided.
func assertMessage(named value.NamedArgsWithDefaults) (string, bool) {
	msg, ok := named.Lookup(names.Message)
	if !ok {
		return "", false
	}
	return string(msg.(value.Str)), true
}

// repr renders a value the way the `repr` built-in does, for diagnostics.
func repr(v value.Value) string {
	r, err := reprImpl(nil, []value.Value{v}, value.NamedArgsWithDefaults{})
	if err != nil {
		return v.Type().String()
	}
	return string(r.(value.Str))
}
