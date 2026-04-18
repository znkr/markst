package ir

import (
	"unique"

	"znkr.io/writst/ir/internal/names"
	"znkr.io/writst/ir/types"
)

// universe is the top-level scope containing all built-in type constructors
// and global functions (range, repr, type). All evaluation scopes inherit
// from universe.
var universe = &scope{
	bindings: map[unique.Handle[string]]Value{
		names.Angle:     reflectedTypes[types.Angle],
		names.Arguments: reflectedTypes[types.Arguments],
		names.Array:     reflectedTypes[types.Array],
		names.Bytes:     reflectedTypes[types.Bytes],
		names.Content:   reflectedTypes[types.Content],
		names.Decimal:   reflectedTypes[types.Decimal],
		names.Float:     reflectedTypes[types.Float],
		names.Fraction:  reflectedTypes[types.Fraction],
		names.Int:       reflectedTypes[types.Int],
		names.Label:     reflectedTypes[types.Label],
		names.Range:     builtinRange,
		names.Ratio:     reflectedTypes[types.Ratio],
		names.Relative:  reflectedTypes[types.Relative],
		names.Repr:      builtinRepr,
		names.Str:       reflectedTypes[types.Str],
		names.Type:      reflectedTypes[types.ReflectedType],
	},
}

// reflectedTypes maps each type constant to its reflected Type value,
// optionally with a constructor function (e.g. int() for type conversion).
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
	types.Float: {
		Reflected:   types.Float,
		Constructor: builtinFloat,
	},
	types.Decimal: {
		Reflected:   types.Decimal,
		Constructor: builtinDecimal,
	},
	types.Length:   {Reflected: types.Length},
	types.Relative: {Reflected: types.Relative},
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
	types.Dict:     {Reflected: types.Dict},
	types.Function: {Reflected: types.Function},
	types.Arguments: {
		Reflected:   types.Arguments,
		Constructor: builtinArguments,
	},
	types.Content: {Reflected: types.Content},
	types.Label: {
		Reflected:   types.Label,
		Constructor: builtinLabel,
	},
}

// typeFields maps each type to its available methods and fields. When a field
// access like x.len() is evaluated, the evaluator looks up the field name in
// typeFields[x.Type()] and returns the method function (pre-bound with x as the
// receiver via [Function.With]).
var typeFields = [...]map[unique.Handle[string]]Value{
	types.None:          {},
	types.ReflectedType: {},
	types.Auto:          {},
	types.Bool:          {},
	types.Int: {
		names.FromBytes: builtinIntFromBytes,
		names.Signum:    builtinSignum,
		names.ToBytes:   builtinIntToBytes,
	},
	types.Float: {
		names.FromBytes:  builtinFloatFromBytes,
		names.Inf:        builtinFloatInf,
		names.IsInfinite: builtinFloatIsInfinite,
		names.IsNan:      builtinFloatIsNan,
		names.Nan:        builtinFloatNan,
		names.Signum:     builtinFloatSignum,
		names.ToBytes:    builtinFloatToBytes,
	},
	types.Decimal:  {},
	types.Length:   {},
	types.Relative: {},
	types.Ratio:    {},
	types.Angle:    {},
	types.Fraction: {},
	types.Str: {
		names.Split: builtinStrSplit,
		names.At:    builtinStrAt,
	},
	types.Bytes: {
		names.Slice: builtinBytesSlice,
	},
	types.Array: {
		names.At:          builtinArrayAt,
		names.Chunks:      builtinArrayChunks,
		names.Enumerate:   builtinArrayEnumerate,
		names.First:       builtinArrayFirst,
		names.Insert:      builtinArrayInsert,
		names.Intersperse: builtinArrayIntersperse,
		names.Join:        builtinArrayJoin,
		names.Last:        builtinArrayLast,
		names.Len:         builtinArrayLen,
		names.Pop:         builtinArrayPop,
		names.Product:     builtinArrayProduct,
		names.Push:        builtinArrayPush,
		names.Remove:      builtinArrayRemove,
		names.Rev:         builtinArrayRev,
		names.Slice:       builtinArraySlice,
		names.Sorted:      builtinArraySorted,
		names.Sum:         builtinArraySum,
		names.ToDict:      builtinArrayToDict,
		names.Windows:     builtinArrayWindows,
		names.Zip:         builtinArrayZip,
	},
	types.Dict: {
		names.At: builtinDictAt,
	},
	types.Function: {},
	types.Arguments: {
		names.Len: builtinArgumentsLen,
		names.At:  builtinArgumentsAt,
	},
	types.Content: {},
}
