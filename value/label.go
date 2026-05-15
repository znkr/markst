package value

import (
	"znkr.io/writst/name"
	"znkr.io/writst/types"
)

// Label is a named marker that can be attached to content elements for
// cross-referencing (e.g. <intro> in Writst markup).
type Label struct {
	Name name.Name
}

func (Label) aValue()           {}
func (*Label) Type() types.Type { return types.Label }
