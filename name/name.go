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

// Package name provides interned string identifiers.
package name

import "unique"

// Name is an interned string handle.
//
// Two Names are equal if and only if they were created from the same string,
// and comparison is O(1) pointer equality.
type Name unique.Handle[string]

var Invalid = Name{}

// Make interns s and returns its Name handle.
//
// Calling Make with the same string always returns an equal Name.
func Make(s string) Name {
	return Name(unique.Make(s))
}

// String returns the underlying string value of the Name.
func (n Name) String() string {
	return unique.Handle[string](n).Value()
}
