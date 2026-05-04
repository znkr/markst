package builtin

import (
	"errors"
	"strconv"
	"unicode/utf8"

	"github.com/woodsbury/decimal128"
	"znkr.io/writst/types"
	"znkr.io/writst/value"
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
