package ir

import (
	"errors"
	"strconv"
	"unicode/utf8"

	"github.com/woodsbury/decimal128"
	"znkr.io/writst/ir/types"
)

var (
	builtinDecimal = &Function{
		Name: "decimal",
		Positional: []Param{
			{Name: "value", Type: types.SetOf(types.Bool, types.Int, types.Float, types.Decimal, types.Str)},
		},
		F: builtinDecimalImpl,
	}
)

func builtinDecimalImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	switch v := args[0].(type) {
	case Bool:
		if v {
			return Decimal(decimal128.FromInt64(1)), nil
		}
		return Decimal(decimal128.FromInt64(0)), nil
	case Int:
		return Decimal(decimal128.FromInt64(int64(v))), nil
	case Float:
		return Decimal(decimal128.FromFloat64(float64(v))), nil
	case Decimal:
		return v, nil
	case Str:
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
				return nil, ArgErrorPosf(0, "invalid decimal: %s", v)
			}
		}
		return Decimal(d), nil
	default:
		panic("should not be reachable due to type checking")
	}
}
