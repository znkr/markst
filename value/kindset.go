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

// KindSet is a set of element kinds, held as one bit per kind. It is what a
// walk filters on: [Preorder] yields the elements whose kind the set contains
// and passes silently through the rest.
type KindSet uint64

// AnyKind is the set of every element kind, the mask for an unfiltered walk.
const AnyKind KindSet = 1<<numElemKinds - 1

// SetOf returns the set holding exactly kinds.
func SetOf(kinds ...ElemKind) KindSet {
	var s KindSet
	for _, k := range kinds {
		s |= 1 << k
	}
	return s
}

// Contains reports whether k is in s.
func (s KindSet) Contains(k ElemKind) bool { return s&(1<<k) != 0 }

// Add returns s with kinds added.
func (s KindSet) Add(kinds ...ElemKind) KindSet { return s | SetOf(kinds...) }

// Remove returns s without kinds — the way to say "everything except", starting
// from [AnyKind].
func (s KindSet) Remove(kinds ...ElemKind) KindSet { return s &^ SetOf(kinds...) }
