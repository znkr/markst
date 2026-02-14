package ir

import (
	"unique"

	"znkr.io/writst/ir/internal/names"
	"znkr.io/writst/ir/types"
)

var universe = &scope{
	bindings: map[unique.Handle[string]]Value{
		names.Bytes: reflectedTypes[types.Bytes],
		names.Int:   reflectedTypes[types.Int],
		names.Repr:  builtinRepr,
		names.Type:  reflectedTypes[types.ReflectedType],
	},
}

var reflectedTypes = [...]*Type{
	types.None: {Reflects: types.None},
	types.ReflectedType: {
		Reflects:    types.ReflectedType,
		Constructor: builtinType,
	},
	types.Auto: {Reflects: types.Auto},
	types.Bool: {Reflects: types.Bool},
	types.Int: {
		Reflects:    types.Int,
		Constructor: builtinInt,
	},
	types.Float:    {Reflects: types.Float},
	types.Length:   {Reflects: types.Length},
	types.Ratio:    {Reflects: types.Ratio},
	types.Angle:    {Reflects: types.Angle},
	types.Fraction: {Reflects: types.Fraction},
	types.String:   {Reflects: types.String},
	types.Bytes: {
		Reflects:    types.Bytes,
		Constructor: builtinBytes,
	},
	types.Array:     {Reflects: types.Array},
	types.Dict:      {Reflects: types.Dict},
	types.Function:  {Reflects: types.Function},
	types.Arguments: {Reflects: types.Arguments},
	types.Content:   {Reflects: types.Content},
}

var methods = [...]map[unique.Handle[string]]*Function{
	types.None:          {},
	types.ReflectedType: {},
	types.Auto:          {},
	types.Bool:          {},
	types.Int: {
		names.FromBytes: builtinIntFromBytes,
		names.Signum:    builtinSignum,
		names.ToBytes:   builtinIntToBytes,
	},
	types.Float:     {},
	types.Length:    {},
	types.Ratio:     {},
	types.Angle:     {},
	types.Fraction:  {},
	types.String:    {},
	types.Bytes:     {},
	types.Array:     {},
	types.Dict:      {},
	types.Function:  {},
	types.Arguments: {},
	types.Content:   {},
}
