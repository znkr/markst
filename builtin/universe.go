// Copyright 2026 Florian Zenker (flo@znkr.io)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package builtin defines everything a Markst document can use without
// declaring it: the global functions and types in [Universe], and the methods
// each type answers to in [TypeFields].
package builtin

import (
	"maps"

	"znkr.io/markst/internal/names"
	"znkr.io/markst/name"
	"znkr.io/markst/types"
	"znkr.io/markst/value"
)

// Std is the `std` module, holding every global binding under one name so a
// document can still reach a builtin it has shadowed.
var Std = &value.Module{
	Name: "std",
	Def:  stdDef,
}

var stdDef = value.SimpleModuleDef{
	names.Angle:      reflectedTypes[types.Angle],
	names.Arguments:  reflectedTypes[types.Arguments],
	names.Array:      reflectedTypes[types.Array],
	names.Assert:     Assert,
	names.Bytes:      reflectedTypes[types.Bytes],
	names.Calc:       Calc,
	names.Content:    reflectedTypes[types.Content],
	names.Datetime:   reflectedTypes[types.Datetime],
	names.Decimal:    reflectedTypes[types.Decimal],
	names.Document:   Document,
	names.Duration:   reflectedTypes[types.Duration],
	names.Emoji:      Emoji,
	names.Emph:       Emph,
	names.Enum:       Enum,
	names.Float:      reflectedTypes[types.Float],
	names.Fraction:   reflectedTypes[types.Fraction],
	names.Function:   reflectedTypes[types.Function],
	names.Footnote:   Footnote,
	names.H:          H,
	names.Heading:    Heading,
	names.Html:       HTML,
	names.Image:      Image,
	names.Int:        reflectedTypes[types.Int],
	names.Label:      reflectedTypes[types.Label],
	names.Linebreak:  Linebreak,
	names.Link:       Link,
	names.List:       List,
	names.Lorem:      Lorem,
	names.Math:       Math,
	names.Metadata:   Metadata,
	names.Par:        Par,
	names.Parbreak:   Parbreak,
	names.Range:      Range,
	names.Ratio:      reflectedTypes[types.Ratio],
	names.Raw:        Raw,
	names.Ref:        Ref,
	names.Relative:   reflectedTypes[types.Relative],
	names.Repr:       Repr,
	names.Smartquote: SmartQuote,
	names.State:      State,
	names.Str:        reflectedTypes[types.Str],
	names.Strong:     Strong,
	names.Sym:        Sym,
	names.Symbol:     Symbol,
	names.Table:      Table,
	names.Terms:      Terms,
	names.Text:       Text,
	names.Underline:  UnderlineMarkup,
	names.Type:       reflectedTypes[types.ReflectedType],
}

// Universe is the scope every document starts in: the type constructors, the
// global functions such as `range` and `repr`, and [Std] itself. A document's
// own bindings shadow these.
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
	types.Datetime: {
		Reflected:   types.Datetime,
		Constructor: Datetime,
	},
	types.Duration: {
		Reflected:   types.Duration,
		Constructor: Duration,
	},
}

// TypeFields holds the methods and fields of each type, indexed by
// [types.Type]. Evaluating `x.len()` looks up `len` in `TypeFields[x.Type()]`
// and gets back the method with x already bound as its receiver.
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
	types.Datetime: {
		names.Day:       DatetimeDay,
		names.Display:   DatetimeDisplay,
		names.Hour:      DatetimeHour,
		names.Minute:    DatetimeMinute,
		names.Month:     DatetimeMonth,
		names.Ordinal:   DatetimeOrdinal,
		names.ParseDate: DatetimeParseDate,
		names.Second:    DatetimeSecond,
		names.Today:     DatetimeToday,
		names.Weekday:   DatetimeWeekday,
		names.Year:      DatetimeYear,
	},
	types.Duration: {
		names.Days:    DurationDays,
		names.Hours:   DurationHours,
		names.Minutes: DurationMinutes,
		names.Seconds: DurationSeconds,
		names.Weeks:   DurationWeeks,
	},
}
