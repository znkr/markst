package ir

import (
	"strings"

	"znkr.io/writst/ir/types"
)

var (
	builtinBytes = &Function{
		Name:          "bytes",
		NumPositional: 1,
		F:             builtinBytesImpl,
	}
)

func builtinBytesImpl(args []Value, named NamedArgsWithDefaults) (Value, error) {
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
}
