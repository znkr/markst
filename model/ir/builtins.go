package ir

import (
	"fmt"
	"strconv"
	"unicode/utf8"
	"unique"
)

var uBase = unique.Make("base")

var builtins = map[unique.Handle[string]]Value{
	unique.Make("int"): &Function{
		Name:          "int",
		NumPositional: 1,
		Defaults: defaultArgs(NamedArgs{
			uBase: Int(10),
		}),
		F: builtinInt,
	},
}

func defaultArgs(named NamedArgs) *Arguments {
	return &Arguments{
		Named: named,
	}
}

func builtinInt(args *Arguments) Value {
	switch v := args.Positional[0].(type) {
	case Bool:
		if v {
			return Int(1)
		}
		return Int(0)
	case Int:
		return v
	case Float:
		return Int(v)
	case String:
		s := string(v)
		base := int(args.Named[uBase].(Int))
		if v[0] > 127 {
			ch, chw := utf8.DecodeRuneInString(s)
			if ch == '\u2212' { // Unicode minus sign
				s = "-" + string(s[chw:])
			}
		}
		n, err := strconv.ParseInt(s, base, 64)
		if err != nil {
			panic(err.Error())
		}
		return Int(n)
	default:
		panic(fmt.Sprintf("cannot convert %s to int", args.Positional[0].Kind()))
	}
}
