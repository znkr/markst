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

// Package types enumerates the Markst value types.
//
// Each [Type] is one type a value can have. A [Set] is a group of them, which
// is how a function parameter says what it accepts.
package types

import (
	"fmt"
	"math/bits"
	"strings"
)

// Type is one Markst value type.
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
	State
	Selector
	Datetime
	Duration
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
	State:         "state",
	Selector:      "selector",
	Datetime:      "datetime",
	Duration:      "duration",
}

// String returns the type's name as diagnostics spell it: "integer", "string",
// "dictionary". It panics if t is not a defined type.
func (t Type) String() string {
	if int(t) < 0 || int(t) >= len(types) {
		panic(fmt.Sprintf("invalid type: %d", t))
	}
	return types[t]
}

// Set is a group of [Type]s, as a bitset. A function parameter uses one to say
// which types it accepts.
type Set uint32

// Any is the set of every defined type.
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
	State,
	Selector,
	Datetime,
	Duration,
)

// SetOf returns the set holding exactly the given types.
func SetOf(types ...Type) Set {
	var s Set
	for _, t := range types {
		s |= 1 << t
	}
	return s
}

// Contains reports whether t is in the set.
func (s Set) Contains(t Type) bool {
	return s&(1<<t) != 0
}

// String returns the types as diagnostics list them: "integer",
// "integer or string", "integer, string, or array".
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
