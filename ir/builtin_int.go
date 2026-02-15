package ir

import (
	"cmp"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strconv"
	"unicode/utf8"

	"znkr.io/writst/ir/internal/names"
	"znkr.io/writst/ir/types"
)

var (
	builtinInt = &Function{
		Name:       "int",
		Positional: []types.Set{types.SetOf(types.Bool, types.Int, types.Float, types.Str, types.Decimal)},
		Named: NamedParams{
			names.Base: NamedParam{Type: types.SetOf(types.Int), Default: Int(10)},
		},
		F: builtinIntImpl,
	}

	builtinIntFromBytes = &Function{
		Name:       "int.from-bytes",
		Positional: []types.Set{types.SetOf(types.Bytes)},
		Named: NamedParams{
			names.Endian: NamedParam{Type: types.SetOf(types.Str), Default: Str("little")},
			names.Signed: NamedParam{Type: types.SetOf(types.Bool), Default: Bool(true)},
		},
		F: builtinIntFromBytesImpl,
	}

	builtinIntToBytes = &Function{
		Name:       "int.to-bytes",
		Positional: []types.Set{types.SetOf(types.Int)},
		Named: NamedParams{
			names.Endian: NamedParam{Type: types.SetOf(types.Str), Default: Str("little")},
			names.Size:   NamedParam{Type: types.SetOf(types.Int), Default: Int(8)},
		},
		F: builtinIntToBytesImpl,
	}

	builtinSignum = &Function{
		Name:       "int.signum",
		Positional: []types.Set{types.SetOf(types.Int)},
		F:          builtinSignumImpl,
	}
)

func builtinIntImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	if args[0].Type() != types.Str && named.IsSet(names.Base) {
		return nil, ArgErrorNamedf(names.Base, "base is only supported for strings")
	}
	switch v := args[0].(type) {
	case Bool:
		if v {
			return Int(1), nil
		}
		return Int(0), nil
	case Int:
		return v, nil
	case Float:
		if v >= math.MaxInt64 || v <= math.MinInt64 {
			return nil, ArgErrorPosf(0, "number too large")
		}
		return Int(v), nil
	case Str:
		s := string(v)
		if s == "" {
			return nil, ArgErrorPosf(0, "string must not be empty")
		}
		base := int(named.Get(names.Base).(Int))
		if base < 2 || base > 36 {
			return nil, ArgErrorNamedf(names.Base, "base must be between 2 and 36")
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
					return nil, ArgErrorPosf(0, "string contains invalid digits")
				} else {
					return nil, ArgErrorPosf(0, "string contains invalid digits for a base %d integer", base)
				}
			case errors.Is(err, strconv.ErrRange):
				largeorsmall := "large"
				if s[0] == '-' {
					largeorsmall = "small"
				}
				err := ArgErrorPosf(0, "integer value is too %s", largeorsmall)
				err.Hint("value does not fit into a signed 64-bit integer")
				err.Hint("try using a floating point number")
				return nil, err
			default:
				panic(err.Error())
			}
		}
		return Int(n), nil
	default:
		return nil, ArgErrorPosf(0, "expected integer, boolean, float, decimal, or string, found %s", args[0].Type())
	}
}

var byteOrder = map[string]binary.ByteOrder{
	"little": binary.LittleEndian,
	"big":    binary.BigEndian,
}

func builtinIntFromBytesImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	b := []byte(args[0].(Bytes))
	order := byteOrder[string(named.Get(names.Endian).(Str))]
	signed := named.Get(names.Signed).(Bool)
	if order == nil {
		return nil, ArgErrorNamedf(names.Endian, "endian must be 'little' or 'big'")
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
		return Int(int64(n)), nil
	} else {
		if n > math.MaxInt64 {
			return nil, ArgErrorPosf(0, "unsigned integer value is too large")
		}
		return Int(n), nil
	}
}

func builtinIntToBytesImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	n := uint64(args[0].(Int))
	order := byteOrder[string(named.Get(names.Endian).(Str))]
	size := int(named.Get(names.Size).(Int))
	if order == nil {
		return nil, ArgErrorNamedf(names.Endian, "endian must be 'little' or 'big'")
	}
	if size < 0 || size > 8 {
		return nil, ArgErrorNamedf(names.Size, "size must be between 1 and 8")
	}
	out := order.(binary.AppendByteOrder).AppendUint64(nil, n)
	if order == binary.LittleEndian {
		return Bytes(out[:size]), nil
	} else {
		return Bytes(out[8-size:]), nil
	}
}

func builtinSignumImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	n := args[0].(Int)
	return Int(cmp.Compare(n, 0)), nil
}
