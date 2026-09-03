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
	"znkr.io/markst/name"
	"znkr.io/markst/types"
	"znkr.io/markst/value"
)

var (
	Label = &value.Function{
		Name: "label",
		Positional: []value.Param{
			{Name: "name", Type: types.SetOf(types.Str)},
		},
		F: labelImpl,
	}
)

func labelImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	switch v := args[0].(type) {
	case value.Str:
		if v == "" {
			return nil, value.ArgErrorPosf(0, "label name must not be empty")
		}
		return &value.Label{Name: name.Make(string(v))}, nil
	default:
		panic("unexpected type: " + v.Type().String())
	}
}
