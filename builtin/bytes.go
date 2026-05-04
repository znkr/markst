package builtin

import (
	"strings"

	"znkr.io/writst/internal/names"
	"znkr.io/writst/types"
	"znkr.io/writst/value"
)

var (
	Bytes = &value.Function{
		Name: "bytes",
		Positional: []value.Param{
			{Name: "value", Type: types.Any},
		},
		F: bytesImpl,
	}

	BytesSlice = &value.Function{
		Name: "bytes.slice",
		Positional: []value.Param{
			{Name: "self", Type: types.SetOf(types.Bytes)},
			{Name: "start", Type: types.SetOf(types.Int)},
			{Name: "end", Type: types.SetOf(types.Int, types.None), Default: value.None{}},
		},
		Named: value.NamedParams{
			names.Count: value.Param{Name: "count", Type: types.SetOf(types.Int)},
		},
		F: bytesSliceImpl,
	}
)

func bytesImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	switch v := args[0].(type) {
	case value.Str:
		return value.Bytes(v), nil
	case *value.Array:
		var sb strings.Builder
		for i, v := range v.Elems {
			if v.Type() != types.Int {
				return nil, value.ArgErrorPosf(0, "expected array of int, got array of %s (element %d)", v.Type(), i)
			}
			sb.WriteByte(byte(v.(value.Int)))
		}
		return value.Bytes(sb.String()), nil
	default:
		return nil, value.ArgErrorPosf(0, "expected string, array, or bytes, found %s", v.Type())
	}
}

func bytesSliceImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	bytes := args[0].(value.Bytes)

	start := args[1].(value.Int)
	if start < 0 {
		start += value.Int(len(bytes))
	}

	var end value.Int
	switch v := args[2].(type) {
	case value.Int:
		if named.IsSet(names.Count) {
			return nil, value.ArgErrorNamedf(names.Count, "count is only supported when end is not specified")
		}
		end = v
		if end < 0 {
			end += value.Int(len(bytes))
		}
	case value.None:
		if count := named.Get(names.Count); count != (value.None{}) {
			count := count.(value.Int)
			if count < 0 {
				return nil, value.ArgErrorNamedf(names.Count, "count must be non-negative")
			}
			end = start + count
		} else {
			end = value.Int(len(bytes))
		}
	default:
		panic("should never happen")
	}

	end = max(start, min(end, value.Int(len(bytes))))
	return value.Bytes(bytes[start:end]), nil
}
