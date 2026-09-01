package builtin

import (
	"fmt"
	"math"

	"github.com/woodsbury/decimal128"
	"znkr.io/markst/internal/names"
	"znkr.io/markst/types"
	"znkr.io/markst/value"
)

var Calc = &value.Module{
	Name: "calc",
	Def: value.SimpleModuleDef{
		names.Inf:   value.Float(math.Inf(1)),
		names.Nan:   value.Float(math.NaN()),
		names.Even:  CalcEven,
		names.Odd:   CalcOdd,
		names.Rem:   CalcRem,
		names.Round: CalcRound,
		names.Sin:   CalcSin,
		names.Min:   CalcMin,
		names.Max:   CalcMax,
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

var CalcMin = &value.Function{
	Name: "calc.min",
	Positional: []value.Param{
		{Name: "values", Type: types.Any},
	},
	Sink: new(0),
	F:    calcMinImpl,
}

var CalcMax = &value.Function{
	Name: "calc.max",
	Positional: []value.Param{
		{Name: "values", Type: types.Any},
	},
	Sink: new(0),
	F:    calcMaxImpl,
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

func calcMinImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	vs := args[0].(*value.Arguments)
	if len(vs.Positional) == 0 {
		return nil, fmt.Errorf("expected at least one value")
	}

	min := vs.Positional[0]
	for i, arg := range vs.Positional[1:] {
		cmp, err := value.Compare(min, arg)
		if err != nil {
			return nil, value.ArgErrorPosf(i+1, "%s", err)
		}
		if cmp > 0 {
			min = arg
		}
	}
	return min, nil
}

func calcMaxImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	vs := args[0].(*value.Arguments)
	if len(vs.Positional) == 0 {
		return nil, fmt.Errorf("expected at least one value")
	}

	max := vs.Positional[0]
	for i, arg := range vs.Positional[1:] {
		cmp, err := value.Compare(max, arg)
		if err != nil {
			return nil, value.ArgErrorPosf(i+1, "%s", err)
		}
		if cmp < 0 {
			max = arg
		}
	}
	return max, nil
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
