package builtin

import (
	"cmp"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strconv"
	"unicode/utf8"

	"github.com/woodsbury/decimal128"
	"znkr.io/markst/internal/names"
	"znkr.io/markst/types"
	"znkr.io/markst/value"
)

var (
	Int = &value.Function{
		Name: "int",
		Positional: []value.Param{
			{Name: "value", Type: types.SetOf(types.Bool, types.Int, types.Float, types.Str, types.Decimal)},
		},
		Named: value.NamedParams{
			names.Base: value.Param{Name: "base", Type: types.SetOf(types.Int), Default: value.Int(10)},
		},
		F: intImpl,
	}

	IntFromBytes = &value.Function{
		Name: "int.from-bytes",
		Positional: []value.Param{
			{Name: "bytes", Type: types.SetOf(types.Bytes)},
		},
		Named: value.NamedParams{
			names.Endian: value.Param{Name: "endian", Type: types.SetOf(types.Str), Default: value.Str("little")},
			names.Signed: value.Param{Name: "signed", Type: types.SetOf(types.Bool), Default: value.Bool(true)},
		},
		F: intFromBytesImpl,
	}

	IntToBytes = &value.Function{
		Name: "int.to-bytes",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Int)},
		},
		Named: value.NamedParams{
			names.Endian: value.Param{Name: "endian", Type: types.SetOf(types.Str), Default: value.Str("little")},
			names.Size:   value.Param{Name: "size", Type: types.SetOf(types.Int), Default: value.Int(8)},
		},
		F: intToBytesImpl,
	}

	Signum = &value.Function{
		Name: "int.signum",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Int)},
		},
		F: signumImpl,
	}
)

func intImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	if args[0].Type() != types.Str && named.IsSet(names.Base) {
		return nil, value.ArgErrorNamedf(names.Base, "base is only supported for strings")
	}
	switch v := args[0].(type) {
	case value.Bool:
		if v {
			return value.Int(1), nil
		}
		return value.Int(0), nil
	case value.Int:
		return v, nil
	case value.Float:
		if v >= math.MaxInt64 || v <= math.MinInt64 {
			return nil, value.ArgErrorPosf(0, "number too large")
		}
		return value.Int(v), nil
	case value.Decimal:
		r, ok := decimal128.Decimal(v).Int64()
		if !ok {
			return nil, value.ArgErrorPosf(0, "number too large")
		}
		return value.Int(r), nil
	case value.Str:
		s := string(v)
		if s == "" {
			return nil, value.ArgErrorPosf(0, "string must not be empty")
		}
		base := int(named.Get(names.Base).(value.Int))
		if base < 2 || base > 36 {
			return nil, value.ArgErrorNamedf(names.Base, "base must be between 2 and 36")
		}
		if s[0] > 127 {
			ch, chw := utf8.DecodeRuneInString(s)
			if ch == '\u2212' { // Unicode minus sign
				s = "-" + string(s[chw:])
			}
		}
		n, err := strconv.ParseInt(s, base, 64)
		if err != nil {
			switch {
			case errors.Is(err, strconv.ErrSyntax):
				if base == 10 {
					return nil, value.ArgErrorPosf(0, "string contains invalid digits")
				} else {
					return nil, value.ArgErrorPosf(0, "string contains invalid digits for a base %d integer", base)
				}
			case errors.Is(err, strconv.ErrRange):
				largeorsmall := "large"
				if s[0] == '-' {
					largeorsmall = "small"
				}
				err := value.ArgErrorPosf(0, "integer value is too %s", largeorsmall)
				err.Hint("value does not fit into a signed 64-bit integer")
				err.Hint("try using a floating point number")
				return nil, err
			default:
				panic(err.Error())
			}
		}
		return value.Int(n), nil
	default:
		panic("should not be reachable due to type checking")
	}
}

var byteOrder = map[string]binary.ByteOrder{
	"little": binary.LittleEndian,
	"big":    binary.BigEndian,
}

func intFromBytesImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	b := []byte(args[0].(value.Bytes))
	order := byteOrder[string(named.Get(names.Endian).(value.Str))]
	signed := named.Get(names.Signed).(value.Bool)
	if order == nil {
		return nil, value.ArgErrorNamedf(names.Endian, "endian must be 'little' or 'big'")
	}
	if len(b) > 8 {
		return nil, fmt.Errorf("too many bytes to convert to a 64 bit number")
	}
	if len(b) != 8 {
		b0 := b
		b = make([]byte, 8)
		if order == binary.LittleEndian {
			copy(b[:len(b0)], b0)
			if signed && len(b0) > 0 && b0[len(b0)-1] >= 128 {
				for i := len(b0); i < 8; i++ {
					b[i] = 0xff
				}
			}
		} else {
			copy(b[8-len(b0):], b0)
			if signed && len(b0) > 0 && b0[0] >= 128 {
				for i := 0; i < 8-len(b0); i++ {
					b[i] = 0xff
				}
			}
		}
	}
	n := order.Uint64(b)
	if signed {
		return value.Int(int64(n)), nil
	} else {
		if n > math.MaxInt64 {
			return nil, value.ArgErrorPosf(0, "unsigned integer value is too large")
		}
		return value.Int(n), nil
	}
}

func intToBytesImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	n := uint64(args[0].(value.Int))
	order := byteOrder[string(named.Get(names.Endian).(value.Str))]
	size := int(named.Get(names.Size).(value.Int))
	if order == nil {
		return nil, value.ArgErrorNamedf(names.Endian, "endian must be 'little' or 'big'")
	}
	if size < 0 || size > 8 {
		return nil, value.ArgErrorNamedf(names.Size, "size must be between 1 and 8")
	}
	out := order.(binary.AppendByteOrder).AppendUint64(nil, n)
	if order == binary.LittleEndian {
		return value.Bytes(out[:size]), nil
	} else {
		return value.Bytes(out[8-size:]), nil
	}
}

func signumImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	n := args[0].(value.Int)
	return value.Int(cmp.Compare(n, 0)), nil
}
