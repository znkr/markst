package builtin

import (
	"maps"

	"znkr.io/writst/internal/names"
	"znkr.io/writst/name"
	"znkr.io/writst/types"
	"znkr.io/writst/value"
)

var Std = &value.Module{
	Name: "std",
	Def:  stdDef,
}

var stdDef = value.SimpleModuleDef{
	names.Angle:     reflectedTypes[types.Angle],
	names.Arguments: reflectedTypes[types.Arguments],
	names.Array:     reflectedTypes[types.Array],
	names.Assert:    Assert,
	names.Bytes:     reflectedTypes[types.Bytes],
	names.Calc:      Calc,
	names.Content:   reflectedTypes[types.Content],
	names.Decimal:   reflectedTypes[types.Decimal],
	names.Document:  Document,
	names.Emoji:     Emoji,
	names.Emph:      Emph,
	names.Enum:      Enum,
	names.Float:     reflectedTypes[types.Float],
	names.Fraction:  reflectedTypes[types.Fraction],
	names.Function:  reflectedTypes[types.Function],
	names.Heading:   Heading,
	names.Int:       reflectedTypes[types.Int],
	names.Label:     reflectedTypes[types.Label],
	names.Linebreak: Linebreak,
	names.Link:      Link,
	names.List:      List,
	names.Lorem:     Lorem,
	names.Par:       Par,
	names.Parbreak:  Parbreak,
	names.Range:     Range,
	names.Ratio:     reflectedTypes[types.Ratio],
	names.Raw:       Raw,
	names.Ref:       Ref,
	names.Relative:  reflectedTypes[types.Relative],
	names.Repr:      Repr,
	names.State:     State,
	names.Str:       reflectedTypes[types.Str],
	names.Strong:    Strong,
	names.Sym:       Sym,
	names.Symbol:    Symbol,
	names.Table:     Table,
	names.Terms:     Terms,
	names.Text:      Text,
	names.Type:      reflectedTypes[types.ReflectedType],
}

// Universe is the top-level scope containing all built-in type constructors
// and global functions (range, repr, type).
var Universe = map[name.Name]value.Value{
	names.Std: Std,
}

func init() {
	maps.Copy(Universe, stdDef)
}

// reflectedTypes maps each type constant to its reflected Type value,
// optionally with a constructor function (e.g. int() for type conversion).
var reflectedTypes = [...]*value.Type{
	types.None: {Reflected: types.None},
	types.ReflectedType: {
		Reflected:   types.ReflectedType,
		Constructor: Type,
	},
	types.Auto: {Reflected: types.Auto},
	types.Bool: {Reflected: types.Bool},
	types.Int: {
		Reflected:   types.Int,
		Constructor: Int,
	},
	types.Float: {
		Reflected:   types.Float,
		Constructor: Float,
	},
	types.Decimal: {
		Reflected:   types.Decimal,
		Constructor: Decimal,
	},
	types.Length:   {Reflected: types.Length},
	types.Relative: {Reflected: types.Relative},
	types.Ratio:    {Reflected: types.Ratio},
	types.Angle:    {Reflected: types.Angle},
	types.Fraction: {Reflected: types.Fraction},
	types.Str: {
		Reflected:   types.Str,
		Constructor: Str,
	},
	types.Bytes: {
		Reflected:   types.Bytes,
		Constructor: Bytes,
	},
	types.Array: {
		Reflected:   types.Array,
		Constructor: Array,
	},
	types.Dict:     {Reflected: types.Dict},
	types.Function: {Reflected: types.Function},
	types.Arguments: {
		Reflected:   types.Arguments,
		Constructor: Arguments,
	},
	types.Content: {Reflected: types.Content},
	types.Label: {
		Reflected:   types.Label,
		Constructor: Label,
	},
	types.State: {Reflected: types.State},
}

// TypeFields maps each type to its available methods and fields. When a field
// access like x.len() is evaluated, the evaluator looks up the field name in
// TypeFields[x.Type()] and returns the method function (pre-bound with x as the
// receiver via [Function.With]).
var TypeFields = [...]map[name.Name]value.Value{
	types.None:          {},
	types.ReflectedType: {},
	types.Auto:          {},
	types.Bool:          {},
	types.Int: {
		names.FromBytes: IntFromBytes,
		names.Signum:    Signum,
		names.ToBytes:   IntToBytes,
	},
	types.Float: {
		names.FromBytes:  FloatFromBytes,
		names.Inf:        FloatInf,
		names.IsInfinite: FloatIsInfinite,
		names.IsNan:      FloatIsNan,
		names.Nan:        FloatNan,
		names.Signum:     FloatSignum,
		names.ToBytes:    FloatToBytes,
	},
	types.Decimal:  {},
	types.Length:   {},
	types.Relative: {},
	types.Ratio:    {},
	types.Angle:    {},
	types.Fraction: {},
	types.Str: {
		names.Split: StrSplit,
		names.At:    StrAt,
		names.Len:   StrLen,
		names.Trim:  StrTrim,
	},
	types.Bytes: {
		names.Slice: BytesSlice,
	},
	types.Array: {
		names.At:          ArrayAt,
		names.Chunks:      ArrayChunks,
		names.Dedup:       ArrayDedup,
		names.Enumerate:   ArrayEnumerate,
		names.First:       ArrayFirst,
		names.Filter:      ArrayFilter,
		names.Fold:        ArrayFold,
		names.Insert:      ArrayInsert,
		names.Intersperse: ArrayIntersperse,
		names.Join:        ArrayJoin,
		names.Last:        ArrayLast,
		names.Len:         ArrayLen,
		names.Map:         ArrayMap,
		names.Pop:         ArrayPop,
		names.Position:    ArrayPosition,
		names.Product:     ArrayProduct,
		names.Push:        ArrayPush,
		names.Reduce:      ArrayReduce,
		names.Remove:      ArrayRemove,
		names.Rev:         ArrayRev,
		names.Slice:       ArraySlice,
		names.Sorted:      ArraySorted,
		names.Sum:         ArraySum,
		names.ToDict:      ArrayToDict,
		names.Windows:     ArrayWindows,
		names.Zip:         ArrayZip,
	},
	types.Dict: {
		names.At:     DictAt,
		names.Insert: DictInsert,
		names.Pairs:  DictPairs,
		names.Remove: DictRemove,
	},
	types.Function: {
		names.With: FunctionWith,
	},
	types.Arguments: {
		names.At:     ArgumentsAt,
		names.Filter: ArgumentsFilter,
		names.Len:    ArgumentsLen,
		names.Map:    ArgumentsMap,
		names.Named:  ArgumentsNamed,
		names.Pos:    ArgumentsPos,
	},
	types.Content: {},
	types.State: {
		names.Update: StateUpdate,
	},
}
