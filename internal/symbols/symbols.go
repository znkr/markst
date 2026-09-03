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

// Package symbols provides the Unicode symbols Markst source can name, such as
// `sym.arrow.r` for →.
//
// A symbol is written as a base name plus zero or more modifiers. One name can
// stand for several characters, so a [Variants] holds all of them and
// [Variants.Resolve] narrows the set one modifier at a time.
package symbols

import (
	"slices"

	"znkr.io/markst/name"
)

//go:generate go tool znkr.io/markst/internal/symbols/symbolsgen Symbols data/sym.txt
//go:generate go tool znkr.io/markst/internal/symbols/symbolsgen Emoji data/emoji.txt

// Module is a group of named bindings, such as `sym` or `emoji`. Modules
// nest, so a Module is itself a [Binding].
type Module struct {
	bindings map[name.Name]Binding
}

// Get returns the binding under n and reports whether there is one.
func (m Module) Get(n name.Name) (Binding, bool) {
	b, ok := m.bindings[n]
	return b, ok
}

func (m Module) aBinding() {}

// Binding is what a name in a symbol module stands for: either a nested
// [Module] or the [Variants] of one symbol.
type Binding interface {
	aBinding()
}

// Variants is every character one symbol name can stand for, each with the
// modifiers that select it.
type Variants []Symbol

// Resolve narrows v to the variants carrying the modifier mod, with that
// modifier removed, and reports whether any did. Applying every modifier a
// name was written with leaves the characters that name can mean.
func (v Variants) Resolve(mod name.Name) (Variants, bool) {
	var result Variants
	for _, s := range v {
		if i := slices.Index(s.Mods, mod); i != -1 {
			result = append(result, Symbol{
				Mods:  slices.Concat(s.Mods[:i], s.Mods[i+1:]),
				Value: s.Value,
			})
		}
	}
	if len(result) == 0 {
		return nil, false
	}
	return result, true
}

func (v Variants) aBinding() {}

// Symbol is one character together with the modifiers that select it.
type Symbol struct {
	Mods  []name.Name
	Value string
}
