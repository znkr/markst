package types

import (
	"fmt"
	"math/bits"
	"strings"
)

type Type int

const (
	None Type = iota
	ReflectedType
	Auto
	Int
	Bool
	Float
	Length
	Decimal
	Ratio
	Angle
	Fraction
	Str
	Bytes
	Array
	Dict
	Function
	Arguments
	Content
)

var types = [...]string{
	None:          "none",
	ReflectedType: "type",
	Int:           "integer",
	Auto:          "auto",
	Bool:          "boolean",
	Float:         "float",
	Length:        "length",
	Decimal:       "decimal",
	Ratio:         "ratio",
	Angle:         "angle",
	Fraction:      "fraction",
	Str:           "string",
	Bytes:         "bytes",
	Array:         "array",
	Dict:          "dictionary",
	Function:      "function",
	Arguments:     "arguments",
	Content:       "content",
}

func (t Type) String() string {
	if int(t) < 0 || int(t) >= len(types) {
		panic(fmt.Sprintf("invalid type: %d", t))
	}
	return types[t]
}

type Set uint32

var Any = SetOf(
	None,
	ReflectedType,
	Auto,
	Bool,
	Int,
	Float,
	Length,
	Ratio,
	Angle,
	Fraction,
	Str,
	Bytes,
	Array,
	Dict,
	Function,
	Arguments,
	Content,
)

func SetOf(types ...Type) Set {
	var s Set
	for _, t := range types {
		s |= 1 << t
	}
	return s
}

func (s Set) Contains(t Type) bool {
	return s&(1<<t) != 0
}

func (s Set) String() string {
	n := bits.OnesCount32(uint32(s))
	var sb strings.Builder
	i := 0
	for bit, name := range types {
		if s&(1<<bit) != 0 {
			if n > 2 && i > 0 && i < n-1 {
				sb.WriteString(", ")
			} else if n > 2 && i == n-1 {
				sb.WriteString(", or ")
			} else if n == 2 && i == 1 {
				sb.WriteString(" or ")
			}
			sb.WriteString(name)
			i++
		}
	}
	return sb.String()
}
