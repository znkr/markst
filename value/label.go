package value

import (
	"unique"

	"znkr.io/writst/types"
)

// Label is a named marker that can be attached to content elements for
// cross-referencing (e.g. <intro> in Writst markup).
type Label struct {
	Name unique.Handle[string]
}

func (Label) aValue()           {}
func (*Label) Type() types.Type { return types.Label }
