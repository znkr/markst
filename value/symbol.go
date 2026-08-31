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

// String returns the character the symbol stands for: of the variants left
// after the modifiers written so far, the one carrying the fewest more. Both
// ⊆ and ⫑ (subset.closed.eq) answer to `subset.eq`, and the one that needs no
// further modifier is the one that was asked for.
func (n *Symbol) String() string {
	best := n.Variants[0]
	for _, v := range n.Variants[1:] {
		if len(v.Mods) < len(best.Mods) {
			best = v
		}
	}
	return best.Value
}

func (n *Symbol) Resolve(mod name.Name) (*Symbol, bool) {
	nvs, ok := n.Variants.Resolve(mod)
	if !ok {
		return nil, false
	}
	return &Symbol{
		Variants: nvs,
	}, true
}
