// Copyright 2026 Florian Zenker (flo@znkr.io)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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
