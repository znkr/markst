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
