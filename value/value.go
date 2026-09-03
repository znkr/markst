// Package value defines the runtime values a Markst document is evaluated to:
// scalars, collections, functions, and the content elements a document is
// built from.
//
// Every value implements [Value]. Content elements additionally implement
// [Content], which adds labels and field access.
package value

import (
	"znkr.io/markst/internal/formatter"
	"znkr.io/markst/types"
)

// Value is implemented by every Markst runtime value.
type Value interface {
	// Type returns the Markst type this value belongs to.
	Type() types.Type

	// Equal reports whether v is this value. It compares types strictly, so
	// an Int is never equal to a Float; for the == operator's rules, which
	// promote across numeric types, use the [Equal] function.
	Equal(v Value) bool

	// Format writes the value back as Markst source. See [FormatValue].
	formatter.Formattable

	aValue()
}
