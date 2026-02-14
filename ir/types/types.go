package types

import "fmt"

type Type int

const (
	None Type = iota
	ReflectedType
	Auto
	Bool
	Int
	Float
	Length
	Ratio
	Angle
	Fraction
	String
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
	Auto:          "auto",
	Bool:          "bool",
	Int:           "int",
	Float:         "float",
	Length:        "length",
	Ratio:         "ratio",
	Angle:         "angle",
	Fraction:      "fraction",
	String:        "string",
	Bytes:         "bytes",
	Array:         "array",
	Dict:          "dict",
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
