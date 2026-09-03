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
	"errors"
	"strconv"
	"unicode/utf8"

	"github.com/woodsbury/decimal128"
	"znkr.io/markst/types"
	"znkr.io/markst/value"
)

var (
	Decimal = &value.Function{
		Name: "decimal",
		Positional: []value.Param{
			{Name: "value", Type: types.SetOf(types.Bool, types.Int, types.Float, types.Decimal, types.Str)},
		},
		F: decimalImpl,
	}
)

func decimalImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	switch v := args[0].(type) {
	case value.Bool:
		if v {
			return value.Decimal(decimal128.FromInt64(1)), nil
		}
		return value.Decimal(decimal128.FromInt64(0)), nil
	case value.Int:
		return value.Decimal(decimal128.FromInt64(int64(v))), nil
	case value.Float:
		return value.Decimal(decimal128.FromFloat64(float64(v))), nil
	case value.Decimal:
		return v, nil
	case value.Str:
		s := string(v)
		if s[0] > 127 {
			ch, chw := utf8.DecodeRuneInString(s)
			if ch == '\u2212' { // Unicode minus sign
				s = "-" + string(s[chw:])
			}
		}
		d, err := decimal128.Parse(s)
		if err != nil {
			switch {
			case errors.Is(err, strconv.ErrSyntax):
				return nil, value.ArgErrorPosf(0, "invalid decimal: %s", v)
			}
		}
		return value.Decimal(d), nil
	default:
		panic("should not be reachable due to type checking")
	}
}
