package value

import (
	"slices"

	"znkr.io/writst/internal/symbols"
	"znkr.io/writst/name"
	"znkr.io/writst/types"
)

type Symbol struct {
	Variants symbols.Variants
}

func (n *Symbol) aValue() {}

func (n *Symbol) Type() types.Type { return types.Symbol }
func (n *Symbol) Equal(other Value) bool {
	o, ok := other.(*Symbol)
	if !ok {
		return false
	}
	return slices.EqualFunc(n.Variants, o.Variants, func(a, b symbols.Symbol) bool {
		return a.Value == b.Value && slices.Equal(a.Mods, b.Mods)
	})
}

func (n *Symbol) String() string { return n.Variants[0].Value }

func (n *Symbol) Resolve(mod name.Name) (*Symbol, bool) {
	nvs, ok := n.Variants.Resolve(mod)
	if !ok {
		return nil, false
	}
	return &Symbol{
		Variants: nvs,
	}, true
}
