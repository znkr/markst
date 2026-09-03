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

// Package slab allocates values in blocks. A caller that makes many values of
// one type, all living as long as each other — the nodes of a syntax tree, the
// instructions of a function — pays one allocation per block rather than one
// per value.
package slab

// Block is how many values a block holds. Every block is the same size: a block
// that ends up half used wastes a few kilobytes at most, where sizing them from
// the work ahead would be guessing at a ratio that varies with the input.
const Block = 128

// Of is a slab of values of type T. The zero slab is ready to use, and it is
// not safe for concurrent use.
//
// A block stays alive as long as any value in it does, which for values made
// and dropped together is no worse than allocating them singly — and a slab
// frees nothing before the whole block goes, so it is for values with that
// shape rather than for a general allocator.
type Of[T any] struct {
	block []T
}

// New adds v to the slab and returns a pointer to it. The pointer stays valid
// for as long as the slab does.
func (s *Of[T]) New(v T) *T {
	// A full block is replaced rather than grown: growing would copy the values
	// already handed out, leaving every pointer to them behind in the old
	// block.
	if len(s.block) == cap(s.block) {
		s.block = make([]T, 0, Block)
	}
	s.block = append(s.block, v)
	return &s.block[len(s.block)-1]
}

// Slice returns n contiguous zero values, for a caller that needs a slice from
// the slab rather than a pointer. Its capacity equals its length, so appending
// to it copies rather than overwriting the next values the slab allocates.
func (s *Of[T]) Slice(n int) []T {
	if n == 0 {
		return nil
	}
	if n > cap(s.block)-len(s.block) {
		s.block = make([]T, 0, max(Block, n))
	}
	start := len(s.block)
	s.block = s.block[:start+n]
	return s.block[start : start+n : start+n]
}
