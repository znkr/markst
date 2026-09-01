package value

import "znkr.io/markst/types"

// Type is a reflected Markst type value, used when the type itself is passed
// as a value.
type Type struct {
	Reflected   types.Type
	Constructor *Function
}

func (*Type) aValue()          {}
func (*Type) Type() types.Type { return types.ReflectedType }
