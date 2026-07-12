// Package ordered provides an insertion-ordered map implementation.
package ordered

import "iter"

// Map is a generic map that preserves insertion order when iterating.
// The zero value is ready to use.
//
// Unlike Go's built-in map, iteration order is deterministic and matches
// the order in which keys were first inserted. Updating an existing key
// does not change its position in the iteration order.
type Map[K comparable, V any] struct {
	keys   []K
	values []V
	index  map[K]int
}

// Get retrieves the value for key. Returns the value and true if found,
// or the zero value and false if not present.
func (m *Map[K, V]) Get(key K) (V, bool) {
	idx, ok := m.index[key]
	if !ok {
		return *new(V), false
	}
	return m.values[idx], true
}

// Put inserts or updates a key-value pair. If the key already exists,
// its value is updated but its position in the iteration order is unchanged.
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

// Delete removes key from the map, preserving the relative order of the
// remaining keys. Returns the removed value and true if the key was present,
// or the zero value and false otherwise.
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

// All returns an iterator over all key-value pairs in insertion order.
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

// UnsafeKeys returns the keys in insertion order.
// The returned slice should not be modified by the caller.
func (m *Map[K, V]) UnsafeKeys() []K {
	return m.keys
}

// Len returns the number of key-value pairs in the map.
func (m *Map[K, V]) Len() int {
	return len(m.keys)
}
