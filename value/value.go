// Package value defines the complete runtime value system for Markst.
//
// Every value in the Markst IR implements the [Value] interface, which
// provides a [types.Type], equality comparison, and formatting support.
package value

import (
	"znkr.io/markst/internal/formatter"
	"znkr.io/markst/types"
)

// Value is the interface implemented by all Markst runtime values. Every value
// has a [types.Type] and supports equality comparison.
type Value interface {
	Type() types.Type
	Equal(Value) bool

	formatter.Formattable

	aValue()
}
