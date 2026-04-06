package ir

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/woodsbury/decimal128"
	"znkr.io/writst/ir/types"
)

var (
	builtinRepr = &Function{
		Name:       "repr",
		Positional: []Param{{Type: types.Any}},
		F:          builtinReprImpl,
	}

	builtinType = &Function{
		Name:       "type",
		Positional: []Param{{Type: types.Any}},
		// F is set in init() to avoid an initialization cycle.
	}
)

func init() {
	builtinType.F = builtinTypeImpl
}

func builtinReprImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	switch v := args[0].(type) {
	case None:
		return Str("none"), nil
	case Bool:
		if v {
			return Str("true"), nil
		}
		return Str("false"), nil
	case Int:
		return Str(strconv.FormatInt(int64(v), 10)), nil
	case Float:
		if math.IsInf(float64(v), 1) {
			return Str("float.inf"), nil
		}
		if math.IsInf(float64(v), -1) {
			return Str("-float.inf"), nil
		}
		if math.IsNaN(float64(v)) {
			return Str("float.nan"), nil
		}
		s := Str(strconv.FormatFloat(float64(v), 'f', -1, 64))
		if !strings.Contains(string(s), ".") {
			s += ".0"
		}
		return s, nil
	case Decimal:
		if decimal128.Decimal(v).IsInf(1) {
			return Str("decimal.inf"), nil
		}
		if decimal128.Decimal(v).IsInf(-1) {
			return Str("-decimal.inf"), nil
		}
		if decimal128.Decimal(v).IsNaN() {
			return Str("decimal.nan"), nil
		}
		s := decimal128.Format(decimal128.Decimal(v), 'f', -1)
		return Str(fmt.Sprintf("decimal(\"%s\")", s)), nil
	default:
		panic(fmt.Sprintf("repr() not implemented for type %s", args[0].Type()))
	}
}

func builtinTypeImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	return reflectedTypes[args[0].Type()], nil
}
