package builtin

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"znkr.io/writst/types"
	"znkr.io/writst/value"
)

var (
	Repr = &value.Function{
		Name:       "repr",
		Positional: []value.Param{{Type: types.Any}},
		F:          reprImpl,
	}

	Type = &value.Function{
		Name:       "type",
		Positional: []value.Param{{Type: types.Any}},
		// F is set in init() to avoid an initialization cycle.
	}
)

func init() {
	Type.F = TypeImpl
}

func reprImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	switch v := args[0].(type) {
	case value.None:
		return value.Str("none"), nil
	case value.Bool:
		return value.Str(v.String()), nil
	case value.Int:
		return value.Str(v.String()), nil
	case value.Float:
		if math.IsInf(float64(v), 1) {
			return value.Str("float.inf"), nil
		}
		if math.IsInf(float64(v), -1) {
			return value.Str("-float.inf"), nil
		}
		if math.IsNaN(float64(v)) {
			return value.Str("float.nan"), nil
		}
		s := value.Str(strconv.FormatFloat(float64(v), 'f', -1, 64))
		if !strings.Contains(string(s), ".") {
			s += ".0"
		}
		return s, nil
	case value.Length:
		return value.Str(v.String()), nil
	case value.Angle:
		return value.Str(v.String()), nil
	case value.Relative:
		return value.Str(v.String()), nil
	case value.Ratio:
		return value.Str(v.String()), nil
	case value.Fraction:
		return value.Str(v.String()), nil
	case value.Decimal:
		s := v.String()
		return value.Str(fmt.Sprintf("decimal(\"%s\")", s)), nil
	default:
		panic(fmt.Sprintf("repr() not implemented for type %s", args[0].Type()))
	}
}

func TypeImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	return reflectedTypes[args[0].Type()], nil
}
