package ir

import (
	"errors"
	"math"
	"strconv"
	"unicode/utf8"
	"unique"

	"znkr.io/writst/ir/types"
)

var _base = unique.Make("base")

var builtins = map[unique.Handle[string]]Value{
	unique.Make("int"): &Function{
		Name:          "int",
		NumPositional: 1,
		Defaults: NamedArgs{
			_base: Int(10),
		},
		F: builtinInt,
	},
}

func builtinInt(args []Value, named NamedArgsWithDefaults) (Value, error) {
	if args[0].Type() != types.String && named.IsSet(_base) {
		return nil, ArgErrorNamedf(_base, "base is only supported for strings")
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
	case String:
		s := string(v)
		if s == "" {
			return nil, ArgErrorPosf(0, "string must not be empty")
		}
		base := int(named.Get(_base).(Int))
		if base < 2 || base > 36 {
			return nil, ArgErrorNamedf(_base, "base must be between 2 and 36")
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
