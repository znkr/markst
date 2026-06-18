// Package symbols provides human readable unicode symbols.
package symbols

import (
	"znkr.io/writst/name"
)

//go:generate go tool znkr.io/writst/internal/symbols/symbolsgen Symbols data/sym.txt
//go:generate go tool znkr.io/writst/internal/symbols/symbolsgen Emoji data/emoji.txt

type Module struct {
	bindings map[name.Name]Binding
}

type Binding interface {
	aBinding()
}

type Variants []Symbol

type Symbol struct {
	mods  []name.Name
	value string
}

func (v Variants) aBinding() {}
func (m Module) aBinding()   {}

func (m Module) Get(n name.Name) (Binding, bool) {
	b, ok := m.bindings[n]
	return b, ok
}
