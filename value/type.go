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

import "znkr.io/markst/types"

// Type is a reflected Markst type value, used when the type itself is passed
// as a value.
type Type struct {
	Reflected   types.Type
	Constructor *Function
}

func (*Type) aValue()          {}
func (*Type) Type() types.Type { return types.ReflectedType }
