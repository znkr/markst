package value

import (
	"unique"

	"znkr.io/writst/types"
)

type Module struct {
	Name        string
	Definitions map[unique.Handle[string]]Value
}

func (*Module) aValue()          {}
func (*Module) Type() types.Type { return types.Module }

func (n *Module) Equal(other Value) bool {
	o, ok := other.(*Module)
	if !ok {
		return false
	}
	return n == o
}
