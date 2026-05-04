// Package value defines the complete runtime value system for Writst.
//
// Every value in the Writst IR implements the [Value] interface, which
// provides a [types.Type], equality comparison, and formatting support.
package value

import (
	"znkr.io/writst/internal/formatter"
	"znkr.io/writst/types"
)

// Value is the interface implemented by all Writst runtime values. Every value
// has a [types.Type] and supports equality comparison.
type Value interface {
	Type() types.Type
	Equal(Value) bool

	formatter.Formattable

	aValue()
}
