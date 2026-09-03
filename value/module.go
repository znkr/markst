package value

import (
	"znkr.io/markst/name"
	"znkr.io/markst/types"
)

// Module is a named group of bindings, reached with `.` from Markst source.
type Module struct {
	Name string
	Def  ModuleDef
}

// ModuleDef supplies a [Module]'s bindings. It is an interface so that a
// module can compute its members rather than hold them all, which is how the
// symbol modules avoid materializing thousands of values.
type ModuleDef interface {
	// Get returns the value bound to the name, or nil if the module has no
	// such binding.
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

// SimpleModuleDef is a [ModuleDef] whose bindings are all held in a map. A
// name the map does not hold is not bound.
type SimpleModuleDef map[name.Name]Value

func (def SimpleModuleDef) Get(name name.Name) Value { return def[name] }
