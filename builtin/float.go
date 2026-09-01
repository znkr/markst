package builtin

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/woodsbury/decimal128"
	"znkr.io/markst/internal/names"
	"znkr.io/markst/types"
	"znkr.io/markst/value"
)

var (
	FloatInf = value.Float(math.Inf(1))
	FloatNan = value.Float(math.NaN())

	Float = &value.Function{
		Name: "float",
		Positional: []value.Param{
			{Name: "value", Type: types.SetOf(
				types.Bool,
				types.Int,
				types.Float,
				types.Decimal,
				types.Ratio,
				types.Str,
			)},
		},
		F: floatImpl,
	}

	FloatIsNan = &value.Function{
		Name: "float.is-nan",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Float)},
		},
		F: floatIsNanImpl,
	}

	FloatIsInfinite = &value.Function{
		Name: "float.is-infinite",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Float)},
		},
		F: floatIsInfiniteImpl,
	}

	FloatSignum = &value.Function{
		Name: "float.signum",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Float)},
		},
		F: floatSignumImpl,
	}

	FloatFromBytes = &value.Function{
		Name: "float.from-bytes",
		Positional: []value.Param{
			{Name: "bytes", Type: types.SetOf(types.Bytes)},
		},
		Named: value.NamedParams{
			names.Endian: value.Param{Name: "endian", Type: types.SetOf(types.Str), Default: value.Str("little")},
		},
		F: floatFromBytesImpl,
	}

	FloatToBytes = &value.Function{
		Name: "float.to-bytes",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Float)},
		},
		Named: value.NamedParams{
			names.Endian: value.Param{Name: "endian", Type: types.SetOf(types.Str), Default: value.Str("little")},
			names.Size:   value.Param{Name: "size", Type: types.SetOf(types.Int), Default: value.Int(8)},
		},
		F: floatToBytesImpl,
	}
)

func floatImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	switch v := args[0].(type) {
	case value.Bool:
		if v {
			return value.Float(1), nil
		}
		return value.Float(0), nil
	case value.Int:
		return value.Float(v), nil
	case value.Float:
		return v, nil
	case value.Decimal:
		f := decimal128.Decimal(v).Float64()
		return value.Float(f), nil
	case value.Ratio:
		return value.Float(v), nil
	case value.Str:
		s := string(v)
		switch s {
		case "inf", "+inf", "float.inf":
			return FloatInf, nil
		case "-inf", "-float.inf":
			return -FloatInf, nil
		case "nan", "float.nan":
			return FloatNan, nil
		default:
			// Replace Unicode minus sign with ASCII hyphen-minus. This can occur at the beginning
			// of the string or after an exponent marker.
			s := strings.ReplaceAll(string(v), "\u2212", "-")
			f, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return nil, value.ArgErrorPosf(0, "invalid float: %s", v)
			}
			return value.Float(f), nil
		}
	default:
		panic("should not be reachable due to type checking")
	}
}

func floatIsNanImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	f := float64(args[0].(value.Float))
	return value.Bool(math.IsNaN(f)), nil
}

func floatIsInfiniteImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	f := float64(args[0].(value.Float))
	return value.Bool(math.IsInf(f, 0)), nil
}

func floatSignumImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	f := float64(args[0].(value.Float))
	if math.IsNaN(f) {
		return FloatNan, nil
	}
	return value.Float(math.Copysign(1.0, f)), nil
}

func floatFromBytesImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	b := []byte(args[0].(value.Bytes))
	order := byteOrder[string(named.Get(names.Endian).(value.Str))]
	if order == nil {
		return nil, value.ArgErrorNamedf(names.Endian, "endian must be 'little' or 'big'")
	}
	switch len(b) {
	case 4:
		n := order.Uint32(b)
		return value.Float(math.Float32frombits(n)), nil
	case 8:
		n := order.Uint64(b)
		return value.Float(math.Float64frombits(n)), nil
	default:
		return nil, fmt.Errorf("bytes must have a length of 4 or 8")
	}
}

func floatToBytesImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	n := float64(args[0].(value.Float))
	order := byteOrder[string(named.Get(names.Endian).(value.Str))]
	size := int(named.Get(names.Size).(value.Int))
	if order == nil {
		return nil, value.ArgErrorNamedf(names.Endian, "endian must be 'little' or 'big'")
	}
	switch size {
	case 4:
		b := make([]byte, 4)
		order.PutUint32(b, math.Float32bits(float32(n)))
		return value.Bytes(b), nil
	case 8:
		b := make([]byte, 8)
		order.PutUint64(b, math.Float64bits(n))
		return value.Bytes(b), nil
	default:
		return nil, value.ArgErrorNamedf(names.Size, "size must be either 4 or 8, got %d", size)
	}
}
