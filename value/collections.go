package value

import (
	"znkr.io/markst/internal/ordered"
	"znkr.io/markst/types"
)

// Array is an ordered sequence of values.
type Array struct {
	Elems []Value
}

// Dict is an ordered dictionary mapping string keys to values.
type Dict struct {
	Elems ordered.Map[Str, Value]
}

func (*Array) aValue() {}
func (*Dict) aValue()  {}

func (*Array) Type() types.Type { return types.Array }
func (*Dict) Type() types.Type  { return types.Dict }
