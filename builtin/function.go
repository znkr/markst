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
	"znkr.io/markst/types"
	"znkr.io/markst/value"
)

var FunctionWith = &value.Function{
	Name: "function.with",
	Positional: []value.Param{
		{Name: "self", Type: types.SetOf(types.Function)},
		{Name: "args", Type: types.SetOf(types.Arguments)},
	},
	Sink: new(1),
	F:    functionWithImpl,
}

func functionWithImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	// An element is a function too, but pre-binding drops the element identity:
	// the result is a plain function and cannot be used as a selector.
	var fn *value.Function
	switch f := args[0].(type) {
	case *value.Function:
		fn = f
	case *value.Element:
		fn = &f.Function
	}
	return fn.With(args[1].(*value.Arguments))
}
