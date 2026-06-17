package value

import (
	"znkr.io/writst/name"
	"znkr.io/writst/types"
)

type Module struct {
	Name string
	Def  ModuleDef
}

type ModuleDef interface {
	Get(name.Name) Value
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

type SimpleModuleDef map[name.Name]Value

func (def SimpleModuleDef) Get(name name.Name) Value { return def[name] }
