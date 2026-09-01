// Package symbols provides human readable unicode symbols.
package symbols

import (
	"slices"

	"znkr.io/markst/name"
)

//go:generate go tool znkr.io/markst/internal/symbols/symbolsgen Symbols data/sym.txt
//go:generate go tool znkr.io/markst/internal/symbols/symbolsgen Emoji data/emoji.txt

type Module struct {
	bindings map[name.Name]Binding
}

func (m Module) Get(n name.Name) (Binding, bool) {
	b, ok := m.bindings[n]
	return b, ok
}

func (m Module) aBinding() {}

type Binding interface {
	aBinding()
}

type Variants []Symbol

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

type Symbol struct {
	Mods  []name.Name
	Value string
}
