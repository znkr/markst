package ir

import (
	"strings"

	"znkr.io/writst/ir/internal/names"
	"znkr.io/writst/ir/types"
)

var (
	builtinBytes = &Function{
		Name:       "bytes",
		Positional: []types.Set{types.Any},
		F:          builtinBytesImpl,
	}

	builtinBytesSlice = &Function{
		Name: "bytes.slice",
		Positional: []types.Set{
			types.SetOf(types.Bytes),
			types.SetOf(types.Int),
			types.SetOf(types.Int, types.None),
		},
		Named: NamedParams{
			names.Count: NamedParam{Type: types.SetOf(types.Int)},
		},
		F: builtinBytesSliceImpl,
	}
)

func builtinBytesImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	switch v := args[0].(type) {
	case Str:
		return Bytes(v), nil
	case Array:
		if args[0].Type() != types.Array {
			return nil, ArgErrorPosf(0, "expected array, got %s", args[0].Type())
		}
		var sb strings.Builder
		for i, v := range args[0].(Array) {
			if v.Type() != types.Int {
				return nil, ArgErrorPosf(0, "expected array of int, got array of %s (element %d)", v.Type(), i)
			}
			sb.WriteByte(byte(v.(Int)))
		}
		return Bytes(sb.String()), nil
	default:
		return nil, ArgErrorPosf(0, "expected string, array, or bytes, found %s", v.Type())
	}
}

func builtinBytesSliceImpl(_ *FuncCallContext, args []Value, named NamedArgsWithDefaults) (Value, error) {
	bytes := args[0].(Bytes)

	start := args[1].(Int)
	if start < 0 {
		start += Int(len(bytes))
	}

	var end Int
	switch v := args[2].(type) {
	case Int:
		if named.IsSet(names.Count) {
			return nil, ArgErrorNamedf(names.Count, "count is only supported when end is not specified")
		}
		end = v
		if end < 0 {
			end += Int(len(bytes))
		}
	case None:
		if count := named.Get(names.Count); count != none {
			count := count.(Int)
			if count < 0 {
				return nil, ArgErrorNamedf(names.Count, "count must be non-negative")
			}
			end = start + count
		} else {
			end = Int(len(bytes))
		}
	default:
		panic("should never happen")
	}

	end = max(start, min(end, Int(len(bytes))))
	return Bytes(bytes[start:end]), nil
}
