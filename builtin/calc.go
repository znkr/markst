package builtin

import (
	"math"
	"unique"

	"github.com/woodsbury/decimal128"
	"znkr.io/writst/internal/names"
	"znkr.io/writst/types"
	"znkr.io/writst/value"
)

var Calc = &value.Module{
	Definitions: map[unique.Handle[string]]value.Value{
		names.Inf:   value.Float(math.Inf(1)),
		names.Nan:   value.Float(math.NaN()),
		names.Even:  CalcEven,
		names.Odd:   CalcOdd,
		names.Rem:   CalcRem,
		names.Round: CalcRound,
		names.Sin:   CalcSin,
	},
}

var CalcEven = &value.Function{
	Name: "calc.even",
	Positional: []value.Param{
		{Name: "value", Type: types.SetOf(types.Int)},
	},
	F: calcEvenImpl,
}

var CalcOdd = &value.Function{
	Name: "calc.odd",
	Positional: []value.Param{
		{Name: "value", Type: types.SetOf(types.Int)},
	},
	F: calcOddImpl,
}

var CalcRem = &value.Function{
	Name: "calc.rem",
	Positional: []value.Param{
		{Name: "dividend", Type: types.SetOf(types.Int, types.Float, types.Decimal)},
		{Name: "divisor", Type: types.SetOf(types.Int, types.Float, types.Decimal)},
	},
	F: calcRemImpl,
}

var CalcRound = &value.Function{
	Name: "calc.round",
	Positional: []value.Param{
		{Name: "value", Type: types.SetOf(types.Int, types.Float, types.Decimal)},
	},
	Named: value.NamedParams{
		names.Digits: value.Param{Name: "digits", Type: types.SetOf(types.Int), Default: value.Int(0)},
	},
	F: calcRoundImpl,
}

var CalcSin = &value.Function{
	Name: "calc.sin",
	Positional: []value.Param{
		{Name: "angle", Type: types.SetOf(types.Int, types.Float, types.Angle)},
	},
	F: calcSinImpl,
}

func calcEvenImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	n := args[0].(value.Int)
	return value.Bool(n%2 == 0), nil
}

func calcOddImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	n := args[0].(value.Int)
	return value.Bool(n%2 != 0), nil
}

func calcRemImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	switch dividend := args[0].(type) {
	case value.Int:
		divisor := args[1].(value.Int)
		if divisor == 0 {
			return nil, value.ArgErrorPosf(1, "divisor must not be zero")
		}
		return value.Int(int64(dividend) % int64(divisor)), nil
	case value.Float:
		divisor := args[1].(value.Float)
		return value.Float(math.Mod(float64(dividend), float64(divisor))), nil
	case value.Decimal:
		divisor := args[1].(value.Decimal)
		_, rem := decimal128.Decimal(dividend).QuoRem(decimal128.Decimal(divisor))
		return value.Decimal(rem), nil
	default:
		panic("unreachable")
	}
}

func calcRoundImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	digits := int64(named.Get(names.Digits).(value.Int))
	switch v := args[0].(type) {
	case value.Int:
		if digits >= 0 {
			return v, nil
		}
		// Round to nearest 10^(-digits)
		factor := int64(math.Pow(10, float64(-digits)))
		n := int64(v)
		return value.Int((n + factor/2) / factor * factor), nil
	case value.Float:
		f := float64(v)
		factor := math.Pow(10, float64(digits))
		return value.Float(math.Round(f*factor) / factor), nil
	case value.Decimal:
		d := decimal128.Decimal(v)
		return value.Decimal(d.Round(int(digits), decimal128.ToNearestAway)), nil
	default:
		panic("unreachable")
	}
}

func calcSinImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	var radians float64
	switch v := args[0].(type) {
	case value.Int:
		radians = float64(v)
	case value.Float:
		radians = float64(v)
	case value.Angle:
		radians = float64(v)
	default:
		panic("unreachable")
	}
	return value.Float(math.Sin(radians)), nil
}
