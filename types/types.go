// Package types defines the Writst type system as an enumeration of value
// types.
//
// Each [Type] constant corresponds to a Writst value type (e.g. Int for
// integers, Str for strings, Content for document content). [Set] provides a
// compact bitset representation for expressing which types a function parameter
// accepts.
package types

import (
	"fmt"
	"math/bits"
	"strings"
)

// Type identifies a Writst value type. The [String] method returns the
// human-readable name used in error messages (e.g. "integer", "string").
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
	Relative
	Ratio
	Angle
	Fraction
	Str
	Symbol
	Bytes
	Array
	Dict
	Function
	Arguments
	Content
	Label
	Module
	Error
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
	Relative:      "relative length",
	Ratio:         "ratio",
	Angle:         "angle",
	Fraction:      "fraction",
	Str:           "string",
	Symbol:        "symbol",
	Bytes:         "bytes",
	Array:         "array",
	Dict:          "dictionary",
	Function:      "function",
	Arguments:     "arguments",
	Content:       "content",
	Label:         "label",
	Module:        "module",
	Error:         "error",
}

func (t Type) String() string {
	if int(t) < 0 || int(t) >= len(types) {
		panic(fmt.Sprintf("invalid type: %d", t))
	}
	return types[t]
}

// Set is a bitset of [Type] values, used to specify which types a function
// parameter accepts. Its [String] method formats the set as a human-readable
// list (e.g. "integer, string, or array").
type Set uint32

// Any is a set containing all defined types.
var Any = SetOf(
	None,
	ReflectedType,
	Auto,
	Decimal,
	Bool,
	Int,
	Float,
	Length,
	Relative,
	Ratio,
	Angle,
	Fraction,
	Str,
	Symbol,
	Bytes,
	Array,
	Dict,
	Function,
	Arguments,
	Content,
	Label,
	Module,
)

// SetOf creates a Set containing the given types.
func SetOf(types ...Type) Set {
	var s Set
	for _, t := range types {
		s |= 1 << t
	}
	return s
}

// Contains reports whether the set contains the given type.
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
