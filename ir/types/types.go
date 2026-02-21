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
	Bool
	Int
	Float
	Decimal
	Length
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
	Label
)

var types = [...]string{
	None:          "none",
	ReflectedType: "type",
	Auto:          "auto",
	Int:           "integer",
	Bool:          "boolean",
	Float:         "float",
	Decimal:       "decimal",
	Length:        "length",
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
	Label:         "label",
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
	Decimal,
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
	Label,
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
	for t, name := range types {
		if s&(1<<t) == 0 {
			continue
		}
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
	return sb.String()
}
