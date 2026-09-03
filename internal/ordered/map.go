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

// Package ordered provides a map that iterates in insertion order.
package ordered

import "iter"

// Map is a map that iterates in the order keys were first inserted, rather
// than the random order a Go map gives. Updating a key does not move it. The
// zero Map is ready to use.
type Map[K comparable, V any] struct {
	keys   []K
	values []V
	index  map[K]int
}

// Get returns the value for key, and reports whether the map holds it. A key
// the map does not hold gives the zero value.
func (m *Map[K, V]) Get(key K) (V, bool) {
	idx, ok := m.index[key]
	if !ok {
		return *new(V), false
	}
	return m.values[idx], true
}

// Put sets the value for key, adding it at the end of the iteration order if
// it is new and leaving it where it is if it is not.
func (m *Map[K, V]) Put(key K, value V) {
	idx, ok := m.index[key]
	if ok {
		m.values[idx] = value
	} else {
		if m.index == nil {
			m.index = make(map[K]int)
		}
		idx = len(m.keys)
		m.keys = append(m.keys, key)
		m.values = append(m.values, value)
		m.index[key] = idx
	}
}

// Delete removes key, leaving the order of the other keys as it was. It
// returns the value that was there and reports whether the key was present.
func (m *Map[K, V]) Delete(key K) (V, bool) {
	idx, ok := m.index[key]
	if !ok {
		return *new(V), false
	}
	removed := m.values[idx]
	m.keys = append(m.keys[:idx], m.keys[idx+1:]...)
	m.values = append(m.values[:idx], m.values[idx+1:]...)
	delete(m.index, key)
	// Reindex the keys that shifted down.
	for i := idx; i < len(m.keys); i++ {
		m.index[m.keys[i]] = i
	}
	return removed, true
}

// All iterates the map's pairs in insertion order.
func (m *Map[K, V]) All() iter.Seq2[K, V] {
	return func(yield func(K, V) bool) {
		for i, k := range m.keys {
			v := m.values[i]
			if !yield(k, v) {
				break
			}
		}
	}
}

// UnsafeKeys returns the keys in insertion order. The slice belongs to the map
// and must not be modified.
func (m *Map[K, V]) UnsafeKeys() []K {
	return m.keys
}

// Len returns how many pairs the map holds.
func (m *Map[K, V]) Len() int {
	return len(m.keys)
}
