package value

import (
	"slices"

	"znkr.io/markst/internal/symbols"
	"znkr.io/markst/name"
	"znkr.io/markst/types"
)

// Symbol is a named Unicode character, such as `arrow.r` for →. A name can
// stand for several characters, so a Symbol holds every variant still
// possible after the modifiers written so far.
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

// String returns the character the symbol stands for: the variant needing the
// fewest further modifiers. Both ⊆ and ⫑ answer to `subset.eq`, and ⊆, which
// needs nothing more, is the one that was asked for.
func (n *Symbol) String() string {
	best := n.Variants[0]
	for _, v := range n.Variants[1:] {
		if len(v.Mods) < len(best.Mods) {
			best = v
		}
	}
	return best.Value
}

// Resolve returns the symbol narrowed to the variants carrying the modifier
// mod, and reports whether any did. `arrow` resolved with `r` is `arrow.r`.
func (n *Symbol) Resolve(mod name.Name) (*Symbol, bool) {
	nvs, ok := n.Variants.Resolve(mod)
	if !ok {
		return nil, false
	}
	return &Symbol{
		Variants: nvs,
	}, true
}
