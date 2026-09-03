package value

import (
	"slices"

	"znkr.io/markst/internal/ordered"
	"znkr.io/markst/name"
	"znkr.io/markst/types"
)

// Arguments holds positional and named arguments for a function call.
type Arguments struct {
	Positional []Value
	Named      NamedArgs
}

// NamedArgs maps argument names to their values at a call site, in the order
// they were written. A spread dict contributes its entries in dict order, at
// the point the spread appears.
type NamedArgs = ordered.Map[name.Name, Value]

// Merge returns the arguments of n followed by those of args: positional
// arguments concatenated, named arguments merged with args winning a
// collision. Either receiver or argument may be nil.
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
	switch {
	case n.Named.Len() == 0:
		named = args.Named
	case args.Named.Len() == 0:
		named = n.Named
	default:
		for k, v := range n.Named.All() {
			named.Put(k, v)
		}
		for k, v := range args.Named.All() {
			named.Put(k, v)
		}
	}
	return &Arguments{
		Positional: pos,
		Named:      named,
	}
}

func (*Arguments) aValue()          {}
func (*Arguments) Type() types.Type { return types.Arguments }
