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

package ordered

import (
	"slices"
	"testing"
)

func TestMap(t *testing.T) {
	t.Run("zero_value_get", func(t *testing.T) {
		var m Map[string, int]
		v, ok := m.Get("missing")
		if ok || v != 0 {
			t.Errorf("Get on empty map: got (%d, %v), want (0, false)", v, ok)
		}
	})

	t.Run("zero_value_len", func(t *testing.T) {
		var m Map[string, int]
		if m.Len() != 0 {
			t.Errorf("Len on empty map: got %d, want 0", m.Len())
		}
	})

	t.Run("zero_value_put", func(t *testing.T) {
		var m Map[string, int]
		m.Put("key", 42)
		v, ok := m.Get("key")
		if !ok || v != 42 {
			t.Errorf("after Put: got (%d, %v), want (42, true)", v, ok)
		}
	})

	t.Run("get_existing", func(t *testing.T) {
		var m Map[string, int]
		m.Put("a", 1)
		v, ok := m.Get("a")
		if !ok || v != 1 {
			t.Errorf("Get existing: got (%d, %v), want (1, true)", v, ok)
		}
	})

	t.Run("get_missing", func(t *testing.T) {
		var m Map[string, int]
		m.Put("a", 1)
		v, ok := m.Get("b")
		if ok || v != 0 {
			t.Errorf("Get missing: got (%d, %v), want (0, false)", v, ok)
		}
	})

	t.Run("update_existing_key", func(t *testing.T) {
		var m Map[string, int]
		m.Put("a", 1)
		m.Put("a", 10)
		v, ok := m.Get("a")
		if !ok || v != 10 {
			t.Errorf("after update: got (%d, %v), want (10, true)", v, ok)
		}
	})

	t.Run("update_preserves_len", func(t *testing.T) {
		var m Map[string, int]
		m.Put("a", 1)
		m.Put("b", 2)
		m.Put("a", 10)
		if m.Len() != 2 {
			t.Errorf("Len after update: got %d, want 2", m.Len())
		}
	})

	t.Run("update_preserves_order", func(t *testing.T) {
		var m Map[string, int]
		m.Put("a", 1)
		m.Put("b", 2)
		m.Put("a", 10)
		var keys []string
		for k := range m.All() {
			keys = append(keys, k)
		}
		if !slices.Equal(keys, []string{"a", "b"}) {
			t.Errorf("iteration order after update: got %v, want [a b]", keys)
		}
	})

	t.Run("insertion_order", func(t *testing.T) {
		var m Map[string, int]
		m.Put("c", 3)
		m.Put("a", 1)
		m.Put("b", 2)

		var keys []string
		var values []int
		for k, v := range m.All() {
			keys = append(keys, k)
			values = append(values, v)
		}

		if !slices.Equal(keys, []string{"c", "a", "b"}) {
			t.Errorf("keys order: got %v, want [c a b]", keys)
		}
		if !slices.Equal(values, []int{3, 1, 2}) {
			t.Errorf("values order: got %v, want [3 1 2]", values)
		}
	})

	t.Run("all_early_break", func(t *testing.T) {
		var m Map[string, int]
		m.Put("a", 1)
		m.Put("b", 2)
		m.Put("c", 3)

		var count int
		for range m.All() {
			count++
			if count == 2 {
				break
			}
		}
		if count != 2 {
			t.Errorf("early break: iterated %d times, want 2", count)
		}
	})

	t.Run("len_increments", func(t *testing.T) {
		var m Map[int, string]
		m.Put(1, "one")
		m.Put(2, "two")
		m.Put(3, "three")
		if m.Len() != 3 {
			t.Errorf("after 3 Puts: got %d, want 3", m.Len())
		}
	})
}
