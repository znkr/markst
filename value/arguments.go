package value

import (
	"maps"
	"slices"
	"unique"

	"znkr.io/writst/types"
)

// Arguments holds positional and named arguments for a function call.
type Arguments struct {
	Positional []Value
	Named      NamedArgs
}

// NamedArgs maps argument names to their values at a call site.
type NamedArgs map[unique.Handle[string]]Value

func (n *Arguments) Merge(args *Arguments) *Arguments {
	if args == nil {
		return n
	}
	if n == nil {
		return args
	}

	var pos []Value
	if len(n.Positional) == 0 {
		pos = args.Positional
	} else if len(args.Positional) == 0 {
		pos = n.Positional
	} else {
		pos = slices.Concat(n.Positional, args.Positional)
	}

	var named NamedArgs
	if len(n.Named) == 0 {
		named = args.Named
	} else if len(args.Named) == 0 {
		named = n.Named
	} else {
		named = maps.Clone(n.Named)
		maps.Copy(named, args.Named)
	}
	return &Arguments{
		Positional: pos,
		Named:      named,
	}
}

func (*Arguments) aValue()          {}
func (*Arguments) Type() types.Type { return types.Arguments }
