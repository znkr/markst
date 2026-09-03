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

package value

import (
	"testing"

	"znkr.io/markst/types"
)

func TestFunctionWith(t *testing.T) {
	nopF := func(_ *FunctionCallContext, _ []Value, _ NamedArgsWithDefaults) (Value, error) {
		return None{}, nil
	}

	rangeFunc := &Function{
		Name: "range",
		Positional: []Param{
			{Type: types.SetOf(types.Int), Default: Int(0)},
			{Type: types.SetOf(types.Int)},
		},
		F: nopF,
	}

	sinkFunc := &Function{
		Name: "anything",
		Positional: []Param{
			{Name: "args", Type: types.SetOf(types.Arguments)},
		},
		Sink: new(0),
		F:    nopF,
	}

	tests := []struct {
		name    string
		fn      *Function
		args    *Arguments
		wantErr bool
	}{
		{"valid_arg", rangeFunc, &Arguments{Positional: []Value{Int(5)}}, false},
		{"two_valid_args", rangeFunc, &Arguments{Positional: []Value{Int(1), Int(2)}}, false},
		{"bad_type_rejected", rangeFunc, &Arguments{Positional: []Value{Str("string")}}, true},
		{"too_many_args", rangeFunc, &Arguments{Positional: []Value{Int(1), Int(2), Int(3)}}, true},
		{"sink_accepts_any_args", sinkFunc, &Arguments{Positional: []Value{Str("ok")}}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.fn.With(tc.args)
			if (err != nil) != tc.wantErr {
				t.Fatalf("With() error = %v, wantErr = %v", err, tc.wantErr)
			}
		})
	}

	t.Run("chained_with", func(t *testing.T) {
		fn, err := rangeFunc.With(&Arguments{Positional: []Value{Int(1)}})
		if err != nil {
			t.Fatalf("first With: %v", err)
		}
		_, err = fn.With(&Arguments{Positional: []Value{Int(2)}})
		if err != nil {
			t.Fatalf("second With: %v", err)
		}
	})
}
