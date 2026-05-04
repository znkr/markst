package value

import "znkr.io/writst/types"

// Type is a reflected Writst type value, used when the type itself is passed
// as a value.
type Type struct {
	Reflected   types.Type
	Constructor *Function
}

func (*Type) aValue()          {}
func (*Type) Type() types.Type { return types.ReflectedType }
