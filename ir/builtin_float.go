package ir

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/woodsbury/decimal128"
	"znkr.io/writst/ir/internal/names"
	"znkr.io/writst/ir/types"
)

var (
	builtinFloatInf = Float(math.Inf(1))
	builtinFloatNan = Float(math.NaN())

	builtinFloat = &Function{
		Name: "float",
		Positional: []Param{
			{Name: "value", Type: types.SetOf(
				types.Bool,
				types.Int,
				types.Float,
				types.Decimal,
				types.Ratio,
				types.Str,
			)},
		},
		F: builtinFloatImpl,
	}

	builtinFloatIsNan = &Function{
		Name: "float.is-nan",
		Positional: []Param{
			{Name: "self", Type: types.SetOf(types.Float)},
		},
		F: builtinFloatIsNanImpl,
	}

	builtinFloatIsInfinite = &Function{
		Name: "float.is-infinite",
		Positional: []Param{
			{Name: "self", Type: types.SetOf(types.Float)},
		},
		F: builtinFloatIsInfiniteImpl,
	}

	builtinFloatSignum = &Function{
		Name: "float.signum",
		Positional: []Param{
			{Name: "self", Type: types.SetOf(types.Float)},
		},
		F: builtinFloatSignumImpl,
	}

	builtinFloatFromBytes = &Function{
		Name: "float.from-bytes",
		Positional: []Param{
			{Name: "bytes", Type: types.SetOf(types.Bytes)},
		},
		Named: NamedParams{
			names.Endian: Param{Name: "endian", Type: types.SetOf(types.Str), Default: Str("little")},
		},
		F: builtinFloatFromBytesImpl,
	}

	builtinFloatToBytes = &Function{
		Name: "float.to-bytes",
		Positional: []Param{
			{Name: "self", Type: types.SetOf(types.Float)},
		},
		Named: NamedParams{
			names.Endian: Param{Name: "endian", Type: types.SetOf(types.Str), Default: Str("little")},
			names.Size:   Param{Name: "size", Type: types.SetOf(types.Int), Default: Int(8)},
		},
		F: builtinFloatToBytesImpl,
	}
)

func builtinFloatImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	switch v := args[0].(type) {
	case Bool:
		if v {
			return Float(1), nil
		}
		return Float(0), nil
	case Int:
		return Float(v), nil
	case Float:
		return v, nil
	case Decimal:
		f := decimal128.Decimal(v).Float64()
		return Float(f), nil
	case Numeric:
		return Float(v.Value), nil
	case Str:
		s := string(v)
		switch s {
		case "inf", "+inf", "float.inf":
			return builtinFloatInf, nil
		case "-inf", "-float.inf":
			return -builtinFloatInf, nil
		case "nan", "float.nan":
			return builtinFloatNan, nil
		default:
			// Replace Unicode minus sign with ASCII hyphen-minus. This can occur at the beginning
			// of the string or after an exponent marker.
			s := strings.ReplaceAll(string(v), "\u2212", "-")
			f, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return nil, ArgErrorPosf(0, "invalid float: %s", v)
			}
			return Float(f), nil
		}
	default:
		panic("should not be reachable due to type checking")
	}
}

func builtinFloatIsNanImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	f := float64(args[0].(Float))
	return Bool(math.IsNaN(f)), nil
}

func builtinFloatIsInfiniteImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	f := float64(args[0].(Float))
	return Bool(math.IsInf(f, 0)), nil
}

func builtinFloatSignumImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	f := float64(args[0].(Float))
	if math.IsNaN(f) {
		return builtinFloatNan, nil
	}
	return Float(math.Copysign(1.0, f)), nil
}

func builtinFloatFromBytesImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	b := []byte(args[0].(Bytes))
	order := byteOrder[string(named.Get(names.Endian).(Str))]
	if order == nil {
		return nil, ArgErrorNamedf(names.Endian, "endian must be 'little' or 'big'")
	}
	switch len(b) {
	case 4:
		n := order.Uint32(b)
		return Float(math.Float32frombits(n)), nil
	case 8:
		n := order.Uint64(b)
		return Float(math.Float64frombits(n)), nil
	default:
		return nil, fmt.Errorf("bytes must have a length of 4 or 8")
	}
}

func builtinFloatToBytesImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	n := float64(args[0].(Float))
	order := byteOrder[string(named.Get(names.Endian).(Str))]
	size := int(named.Get(names.Size).(Int))
	if order == nil {
		return nil, ArgErrorNamedf(names.Endian, "endian must be 'little' or 'big'")
	}
	switch size {
	case 4:
		b := make([]byte, 4)
		order.PutUint32(b, math.Float32bits(float32(n)))
		return Bytes(b), nil
	case 8:
		b := make([]byte, 8)
		order.PutUint64(b, math.Float64bits(n))
		return Bytes(b), nil
	default:
		return nil, ArgErrorNamedf(names.Size, "size must be either 4 or 8, got %d", size)
	}
}
