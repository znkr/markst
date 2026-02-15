package ir

import (
	"unique"

	"znkr.io/writst/ir/internal/names"
	"znkr.io/writst/ir/types"
)

var universe = &scope{
	bindings: map[unique.Handle[string]]Value{
		names.Array: reflectedTypes[types.Array],
		names.Bytes: reflectedTypes[types.Bytes],
		names.Int:   reflectedTypes[types.Int],
		names.Range: builtinRange,
		names.Repr:  builtinRepr,
		names.Str:   reflectedTypes[types.Str],
		names.Type:  reflectedTypes[types.ReflectedType],
	},
}

var reflectedTypes = [...]*Type{
	types.None: {Reflected: types.None},
	types.ReflectedType: {
		Reflected:   types.ReflectedType,
		Constructor: builtinType,
	},
	types.Auto: {Reflected: types.Auto},
	types.Bool: {Reflected: types.Bool},
	types.Int: {
		Reflected:   types.Int,
		Constructor: builtinInt,
	},
	types.Float:    {Reflected: types.Float},
	types.Length:   {Reflected: types.Length},
	types.Ratio:    {Reflected: types.Ratio},
	types.Angle:    {Reflected: types.Angle},
	types.Fraction: {Reflected: types.Fraction},
	types.Str: {
		Reflected:   types.Str,
		Constructor: builtinStr,
	},
	types.Bytes: {
		Reflected:   types.Bytes,
		Constructor: builtinBytes,
	},
	types.Array: {
		Reflected:   types.Array,
		Constructor: builtinArray,
	},
	types.Dict:      {Reflected: types.Dict},
	types.Function:  {Reflected: types.Function},
	types.Arguments: {Reflected: types.Arguments},
	types.Content:   {Reflected: types.Content},
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
	types.Float:    {},
	types.Length:   {},
	types.Ratio:    {},
	types.Angle:    {},
	types.Fraction: {},
	types.Str:      {},
	types.Bytes: {
		names.Slice: builtinBytesSlice,
	},
	types.Array: {
		names.Join: builtinArrayJoin,
	},
	types.Dict:      {},
	types.Function:  {},
	types.Arguments: {},
	types.Content:   {},
}
