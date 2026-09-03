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

package builtin

import (
	"unicode/utf8"

	"znkr.io/markst/internal/names"
	"znkr.io/markst/name"
	"znkr.io/markst/types"
	"znkr.io/markst/value"
)

// Math is the `math` module, exposing the math elements in code mode (e.g.
// `#math.frac(1, 2)`).
var Math = &value.Module{
	Name: "math",
	Def: mathModuleDef{
		names.Accent:    Accent,
		names.Cancel:    Cancel,
		names.Equation:  Equation,
		names.Frac:      Frac,
		names.Lr:        Lr,
		names.Mid:       Mid,
		names.Op:        Op,
		names.Root:      Root,
		names.Sqrt:      Sqrt,
		names.Text:      Text,
		names.Vec:       Vec,
		names.Mat:       Mat,
		names.Cases:     Cases,
		names.Floor:     Floor,
		names.Ceil:      Ceil,
		names.Abs:       Abs,
		names.Norm:      Norm,
		names.Underline: Underline,

		// Font styles (see mathstyle.go).
		names.Bold:    Bold,
		names.Italic:  Italic,
		names.Upright: Upright,
		names.Serif:   Serif,
		names.Sans:    Sans,
		names.Cal:     Cal,
		names.Frak:    Frak,
		names.Mono:    Mono,
		names.Bb:      Bb,
	},
}

// mathModuleDef resolves a name against the math elements first and then falls
// back to the symbol module, so math symbols (e.g. `math.pi`, `math.arrow`) are
// accessible under the `math` namespace just as they are inside an equation.
type mathModuleDef value.SimpleModuleDef

func (m mathModuleDef) Get(n name.Name) value.Value {
	if v := value.SimpleModuleDef(m).Get(n); v != nil {
		return v
	}
	if spec, ok := mathOps[n]; ok {
		// A fresh element per lookup. A label attaches to the value itself, so a
		// shared element would carry a label written in another document.
		return &value.MathOp{Text: &value.MathText{Text: spec.text}, Limits: spec.limits}
	}
	if v, ok := mathSpacing[n]; ok {
		return v
	}
	return Sym.Def.Get(n)
}

// mathSpacing are the spacing constants the math module defines. They can be
// shared, unlike the operators: [value.HSpace] takes no label, so there is
// nothing in one to write to.
var mathSpacing = map[name.Name]*value.HSpace{
	name.Make("thin"):  {Amount: value.Length{Em: 1.0 / 6.0}},
	name.Make("med"):   {Amount: value.Length{Em: 2.0 / 9.0}},
	name.Make("thick"): {Amount: value.Length{Em: 5.0 / 18.0}},
	name.Make("quad"):  {Amount: value.Length{Em: 1}},
	name.Make("wide"):  {Amount: value.Length{Em: 2}},
}

// mathOps are the text operators the math module predefines, taken from
// Typst's list. limits says whether an attachment on the operator belongs
// above and below it in a block equation.
var mathOps = buildMathOps()

type mathOpSpec struct {
	text   string
	limits bool
}

func buildMathOps() map[name.Name]mathOpSpec {
	ops := make(map[name.Name]mathOpSpec)
	for _, op := range []string{
		"arccos", "arcsin", "arctan", "arg", "cos", "cosh", "cot", "coth",
		"csc", "csch", "ctg", "deg", "dim", "exp", "hom", "id", "im", "ker",
		"lg", "ln", "log", "mod", "sec", "sech", "sin", "sinc", "sinh", "tan",
		"tanh", "tg", "tr",
	} {
		ops[name.Make(op)] = mathOpSpec{text: op}
	}
	for _, op := range []string{
		"det", "gcd", "inf", "lcm", "lim", "max", "min", "Pr", "sup",
	} {
		ops[name.Make(op)] = mathOpSpec{text: op, limits: true}
	}
	// The two operators spelled with a space between the words.
	ops[name.Make("liminf")] = mathOpSpec{text: "lim inf", limits: true}
	ops[name.Make("limsup")] = mathOpSpec{text: "lim sup", limits: true}
	return ops
}

// mathContentType is the accepted type of a math element's content argument:
// the types [value.ToMathContent] turns into math text, so that `$frac(sum,
// 2)$` and `#math.frac(1, 2)` work the same as `$frac(x, 2)$` — Typst's Content
// cast accepts none, symbols, strings and numbers too.
//
// It is not the full range [mathContent] can coerce: anything else still falls
// through to [value.ToContent], which renders it as raw text. Those values only
// reach an element through the untyped positional sink of vec/mat/cases.
var mathContentType = types.SetOf(types.Content, types.Symbol, types.Str, types.None, types.Int, types.Float)

// mathContent coerces an argument of a math element to content. Symbols,
// strings and numbers become [value.MathText] rather than the upright
// [value.Text] or [value.Raw] that [value.ToContent] would produce, so a letter
// and a symbol resolving to the same character render alike. That is the rule
// the evaluator applies to math operands; see frame.mathContentOf. The math
// elements are math-only whichever mode they are called from, so this needs no
// context to decide.
//
// The result is never nil: none becomes empty content, as in frame.contentOf.
func mathContent(v value.Value) (value.Content, error) {
	c := value.ToMathContent(v)
	if c == nil {
		return &value.Sequence{}, nil
	}
	return c, nil
}

// mathCells converts a run of argument values into content cells, flattening
// any array values (rows produced by the analyzer for `;`-separated lists).
func mathCells(vals []value.Value, cell func(value.Value) (value.Content, error)) ([]value.Content, error) {
	var out []value.Content
	for i, v := range vals {
		if arr, ok := v.(*value.Array); ok {
			for _, e := range arr.Elems {
				c, err := cell(e)
				if err != nil {
					return nil, value.ArgErrorPosf(i, "%s", err.Error())
				}
				out = append(out, c)
			}
			continue
		}
		c, err := cell(v)
		if err != nil {
			return nil, value.ArgErrorPosf(i, "%s", err.Error())
		}
		out = append(out, c)
	}
	return out, nil
}

// mathContentStrict is [mathContent] for the elements whose cells are declared
// as content — vec and cases — where a number is a mistake rather than
// something to show. mat, whose cells show whatever they are handed, uses
// [mathContent].
func mathContentStrict(v value.Value) (value.Content, error) {
	c, err := value.CastMathContent(v)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return &value.Sequence{}, nil
	}
	return c, nil
}

// delimType is the accepted type of the vec/mat/cases `delim` property: none,
// a string or symbol delimiter, or a (open, close) array pair.
var delimType = types.SetOf(types.None, types.Str, types.Symbol, types.Array)

// validDelims is the set of delimiter glyphs accepted by the vec/mat/cases
// `delim` property.
var validDelims = map[string]bool{
	"(": true, ")": true,
	"[": true, "]": true,
	"{": true, "}": true,
	"⟦": true, "⟧": true,
	"⟨": true, "⟩": true,
	"⌊": true, "⌋": true,
	"⌈": true, "⌉": true,
	"|": true, "‖": true,
}

// validateDelim validates a vec/mat/cases `delim` named argument. delim may be
// none (no delimiter), a single-character string or symbol, or a (open, close)
// pair given as a two-element array; each side must be none or a single valid
// delimiter glyph. The error points at the whole delim value.
func validateDelim(named value.NamedArgs) *value.FunctionCallError {
	delim, ok := named.Get(names.Delim)
	if !ok {
		return nil
	}
	if arr, ok := delim.(*value.Array); ok {
		if len(arr.Elems) != 2 {
			return value.ArgErrorNamedf(names.Delim, "expected 2 delimiters, found %d", len(arr.Elems))
		}
		for _, e := range arr.Elems {
			if err := validateDelimSide(e); err != nil {
				return err
			}
		}
		return nil
	}
	return validateDelimSide(delim)
}

func validateDelimSide(v value.Value) *value.FunctionCallError {
	s, isNone, ok := delimString(v)
	if isNone {
		return nil // none means "no delimiter".
	}
	if !ok {
		// bind type-checks the delim argument itself, but not the individual
		// elements of a pair array, so a bad element type is rejected here.
		return value.ArgErrorNamedf(names.Delim, "expected none, string, symbol, or array, found %s", v.Type())
	}
	if utf8.RuneCountInString(s) != 1 {
		return value.ArgErrorNamedf(names.Delim, "expected exactly one character")
	}
	if !validDelims[s] {
		return value.ArgErrorNamedf(names.Delim, "invalid delimiter: %q", s)
	}
	return nil
}

// delimString extracts the glyph of a delimiter value. ok is false for value
// types that can't denote a delimiter. A math escape such as `\(` arrives as a
// [value.Symbol] (see the analyzer's escape lowering), so no content case is
// needed.
func delimString(v value.Value) (s string, isNone, ok bool) {
	switch x := v.(type) {
	case value.None:
		return "", true, true
	case value.Str:
		return string(x), false, true
	case *value.Symbol:
		return x.String(), false, true
	}
	return "", false, false
}

// Vec is the `vec(a, b, c)` math element; it builds a column [value.MathVec].
var Vec = value.NewElement[*value.MathVec](value.Function{
	Name:       "math.vec",
	Positional: []value.Param{{Name: "children", Type: mathContentType}},
	Sink:       new(0),
	Named: value.NamedParams{
		names.Delim: {Name: "delim", Type: delimType},
		names.Align: {Name: "align", Type: types.Any},
		names.Gap:   {Name: "gap", Type: types.Any},
	},
	Validate: validateDelim,
	F:        vecImpl,
})

func vecImpl(_ *value.FunctionCallContext, args []value.Value, _ value.NamedArgsWithDefaults) (value.Value, error) {
	sink := args[0].(*value.Arguments)
	if err := rejectSinkNamed(sink); err != nil {
		return nil, err
	}
	children, err := mathCells(sink.Positional, mathContentStrict)
	if err != nil {
		return nil, err
	}
	return &value.MathVec{Children: children}, nil
}

// Cases is the `cases(a, b)` math element; it builds a [value.MathCases].
var Cases = value.NewElement[*value.MathCases](value.Function{
	Name:       "math.cases",
	Positional: []value.Param{{Name: "children", Type: mathContentType}},
	Sink:       new(0),
	Named: value.NamedParams{
		names.Delim:   {Name: "delim", Type: delimType},
		names.Reverse: {Name: "reverse", Type: types.SetOf(types.Bool)},
		names.Gap:     {Name: "gap", Type: types.Any},
	},
	Validate: validateDelim,
	F:        casesImpl,
})

func casesImpl(_ *value.FunctionCallContext, args []value.Value, _ value.NamedArgsWithDefaults) (value.Value, error) {
	sink := args[0].(*value.Arguments)
	if err := rejectSinkNamed(sink); err != nil {
		return nil, err
	}
	children, err := mathCells(sink.Positional, mathContentStrict)
	if err != nil {
		return nil, err
	}
	return &value.MathCases{Children: children}, nil
}

// Mat is the `mat(a, b; c, d)` math element; it builds a [value.MathMat]. The
// analyzer groups `;`-separated rows into array arguments; a call without any
// `;` becomes a single row.
var Mat = value.NewElement[*value.MathMat](value.Function{
	Name:       "math.mat",
	Positional: []value.Param{{Name: "rows", Type: mathContentType}},
	Sink:       new(0),
	Named: value.NamedParams{
		names.Delim:     {Name: "delim", Type: delimType},
		names.Align:     {Name: "align", Type: types.Any},
		names.Augment:   {Name: "augment", Type: types.Any},
		names.Gap:       {Name: "gap", Type: types.Any},
		names.RowGap:    {Name: "row-gap", Type: types.Any},
		names.ColumnGap: {Name: "column-gap", Type: types.Any},
	},
	Validate: validateDelim,
	F:        matImpl,
})

func matImpl(_ *value.FunctionCallContext, args []value.Value, _ value.NamedArgsWithDefaults) (value.Value, error) {
	sink := args[0].(*value.Arguments)
	if err := rejectSinkNamed(sink); err != nil {
		return nil, err
	}
	pos := sink.Positional
	grouped := false
	for _, v := range pos {
		if _, ok := v.(*value.Array); ok {
			grouped = true
			break
		}
	}
	var rows [][]value.Content
	if grouped {
		for i, v := range pos {
			arr, ok := v.(*value.Array)
			if !ok {
				c, err := mathContent(v)
				if err != nil {
					return nil, value.ArgErrorPosf(i, "%s", err.Error())
				}
				rows = append(rows, []value.Content{c})
				continue
			}
			row, err := mathCells(arr.Elems, mathContent)
			if err != nil {
				return nil, err
			}
			rows = append(rows, row)
		}
	} else if row, err := mathCells(pos, mathContent); err != nil {
		return nil, err
	} else if len(row) > 0 {
		rows = append(rows, row)
	}
	return &value.MathMat{Rows: rows}, nil
}

// Equation is the `math.equation` element; it builds a [value.Equation].
var Equation = value.NewElement[*value.Equation](value.Function{
	Name: "math.equation",
	Positional: []value.Param{
		{Name: "body", Type: mathContentType},
	},
	Named: value.NamedParams{
		names.Block: {Name: "block", Type: types.SetOf(types.Bool), Default: value.Bool(false)},
		// The font-style properties the math style functions set on a scope (see
		// mathStyle). Typst marks these internal. They are declared so that `#set
		// math.equation(bold: true)`, which never runs equationImpl, can carry them,
		// but an equation has no field to store a style in, so the constructor
		// rejects them (see equationImpl).
		names.Bold:    {Name: "bold", Type: types.SetOf(types.Bool)},
		names.Italic:  {Name: "italic", Type: types.SetOf(types.Bool)},
		names.Variant: {Name: "variant", Type: types.SetOf(types.Str)},
	},
	F: equationImpl,
})

func equationImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	// The font-style properties have meaning only as a set rule, which
	// realization applies to the math leaves below it. Constructing an equation
	// with one would discard it without notice, so it is an error instead.
	for _, field := range mathStyleFields {
		if _, ok := named.Args.Get(field); ok {
			return nil, value.ArgErrorNamedPairf(field, "%s is only settable with a set rule", field)
		}
	}
	body, err := mathContent(args[0])
	if err != nil {
		return nil, value.ArgErrorPosf(0, "%s", err.Error())
	}
	return &value.Equation{Block: bool(named.Get(names.Block).(value.Bool)), Body: body}, nil
}

// Frac is the `frac(num, denom)` math element; it builds a [value.MathFrac].
var Frac = value.NewElement[*value.MathFrac](value.Function{
	Name: "math.frac",
	Positional: []value.Param{
		{Name: "num", Type: mathContentType},
		{Name: "denom", Type: mathContentType},
	},
	F: fracImpl,
})

func fracImpl(_ *value.FunctionCallContext, args []value.Value, _ value.NamedArgsWithDefaults) (value.Value, error) {
	num, err := mathContent(args[0])
	if err != nil {
		return nil, value.ArgErrorPosf(0, "%s", err.Error())
	}
	denom, err := mathContent(args[1])
	if err != nil {
		return nil, value.ArgErrorPosf(1, "%s", err.Error())
	}
	return &value.MathFrac{Num: num, Denom: denom}, nil
}

// Root is the `root(index, radicand)` math element; it builds a
// [value.MathRoot].
var Root = value.NewElement[*value.MathRoot](value.Function{
	Name: "math.root",
	Positional: []value.Param{
		{Name: "index", Type: mathContentType},
		{Name: "radicand", Type: mathContentType},
	},
	F: rootImpl,
})

func rootImpl(_ *value.FunctionCallContext, args []value.Value, _ value.NamedArgsWithDefaults) (value.Value, error) {
	index, err := mathContent(args[0])
	if err != nil {
		return nil, value.ArgErrorPosf(0, "%s", err.Error())
	}
	radicand, err := mathContent(args[1])
	if err != nil {
		return nil, value.ArgErrorPosf(1, "%s", err.Error())
	}
	return &value.MathRoot{Index: index, Radicand: radicand}, nil
}

// Sqrt is `sqrt(radicand)`, which builds a [value.MathRoot] with no index.
//
// It is a plain function rather than an element, as in Typst: what it produces
// is a `math.root`, so a set or show rule targets [Root] instead. Making it an
// element too would give two elements the same content type, and
// [value.Element] matches on that type alone, so `#show math.sqrt:` would also
// rewrite `$root(3, x)$`.
var Sqrt = &value.Function{
	Name: "math.sqrt",
	Positional: []value.Param{
		{Name: "radicand", Type: mathContentType},
	},
	F: sqrtImpl,
}

func sqrtImpl(_ *value.FunctionCallContext, args []value.Value, _ value.NamedArgsWithDefaults) (value.Value, error) {
	radicand, err := mathContent(args[0])
	if err != nil {
		return nil, value.ArgErrorPosf(0, "%s", err.Error())
	}
	return &value.MathRoot{Radicand: radicand}, nil
}

// Lr is the `lr(..)` math element; it builds a [value.MathLr]. Several
// positional arguments are joined with a comma between them, so
// `lr(A dif x, f(x)\))` wraps the whole `A dif x, f(x))`.
var Lr = value.NewElement[*value.MathLr](value.Function{
	Name:       "math.lr",
	Positional: []value.Param{{Name: "body", Type: mathContentType}},
	Sink:       new(0),
	Named:      lrNamedParams(),
	F:          lrImpl,
})

func lrImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	sink := args[0].(*value.Arguments)
	if err := rejectSinkNamed(sink); err != nil {
		return nil, err
	}
	parts, err := mathCells(sink.Positional, mathContent)
	if err != nil {
		return nil, err
	}
	// A single argument is the body itself: `lr(x).body` must equal the `x` that
	// was passed in, so it is not wrapped in a sequence.
	if len(parts) == 1 {
		return newLr(parts[0], named), nil
	}
	var body []value.Content
	for i, p := range parts {
		if i > 0 {
			body = append(body, &value.MathText{Text: ","})
		}
		body = append(body, p)
	}
	return newLr(&value.Sequence{Children: body}, named), nil
}

// lrNamedParams are the properties shared by `math.lr` and the delimiter
// functions built by [lrFunc].
func lrNamedParams() value.NamedParams {
	return value.NamedParams{
		names.Size: {Name: "size", Type: types.SetOf(types.Ratio, types.Length, types.Relative)},
	}
}

// newLr builds the node shared by `math.lr` and the delimiter functions.
func newLr(body value.Content, named value.NamedArgsWithDefaults) *value.MathLr {
	lr := &value.MathLr{Body: body}
	if size, ok := named.Lookup(names.Size); ok {
		lr.Size = size
	}
	return lr
}

// lrFunc builds a math function that wraps its single content argument in the
// given open/close delimiters (e.g. floor → ⌊ ⌋). The delimiters go into the
// body, exactly as [Lr] would have received them.
//
// These are functions rather than elements for the same reason as [Sqrt]: they
// all produce a [value.MathLr], i.e. a `math.lr`, and [value.Element]
// matches on the content type alone — as elements, `#show math.floor:` would
// also rewrite `$abs(x)$`, `$ceil(x)$` and `$norm(x)$`.
func lrFunc(name, open, closing string) *value.Function {
	return &value.Function{
		Name:       name,
		Positional: []value.Param{{Name: "body", Type: mathContentType}},
		Named:      lrNamedParams(),
		F: func(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
			body, err := mathContent(args[0])
			if err != nil {
				return nil, value.ArgErrorPosf(0, "%s", err.Error())
			}
			return newLr(&value.Sequence{Children: []value.Content{
				&value.MathText{Text: open},
				body,
				&value.MathText{Text: closing},
			}}, named), nil
		},
	}
}

// Floor, Ceil, Abs, Norm are delimiter math functions.
var (
	Floor = lrFunc("math.floor", "⌊", "⌋")
	Ceil  = lrFunc("math.ceil", "⌈", "⌉")
	Abs   = lrFunc("math.abs", "|", "|")
	Norm  = lrFunc("math.norm", "‖", "‖")
)

// Cancel is the `cancel(body, angle: ...)` math element; it builds a
// [value.MathCancel]. Only the subset of fields exercised so far is modeled:
// the canceled body and the `angle` field (retained for field access).
var Cancel = value.NewElement[*value.MathCancel](value.Function{
	Name: "math.cancel",
	Positional: []value.Param{
		{Name: "body", Type: mathContentType},
	},
	Named: value.NamedParams{
		names.Angle: {Name: "angle", Type: types.Any},
	},
	F: cancelImpl,
})

func cancelImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	body, err := mathContent(args[0])
	if err != nil {
		return nil, value.ArgErrorPosf(0, "%s", err.Error())
	}
	c := &value.MathCancel{Body: body}
	if angle, ok := named.Lookup(names.Angle); ok {
		c.Angle = angle
	}
	return c, nil
}

// Op is the `op(text, limits: false)` math element; it builds a
// [value.MathOp]. The predefined operators (`sin`, `lim`, …) are the same
// element under a name, see mathOps.
var Op = value.NewElement[*value.MathOp](value.Function{
	Name:       "math.op",
	Positional: []value.Param{{Name: "text", Type: mathContentType}},
	Named: value.NamedParams{
		names.Limits: {Name: "limits", Type: types.SetOf(types.Bool), Default: value.Bool(false)},
	},
	F: opImpl,
})

func opImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	text, err := mathContent(args[0])
	if err != nil {
		return nil, value.ArgErrorPosf(0, "%s", err.Error())
	}
	return &value.MathOp{Text: text, Limits: bool(named.Get(names.Limits).(value.Bool))}, nil
}

// Mid is the `mid(x)` math element; it builds a [value.MathMid]. In Typst it
// marks a delimiter to be scaled by the enclosing `lr` group, which only shows
// up in layout; here it is just the wrapper node.
var Mid = value.NewElement[*value.MathMid](value.Function{
	Name:       "math.mid",
	Positional: []value.Param{{Name: "body", Type: mathContentType}},
	F:          midImpl,
})

func midImpl(_ *value.FunctionCallContext, args []value.Value, _ value.NamedArgsWithDefaults) (value.Value, error) {
	body, err := mathContent(args[0])
	if err != nil {
		return nil, value.ArgErrorPosf(0, "%s", err.Error())
	}
	return &value.MathMid{Body: body}, nil
}

// Underline is the `underline(x)` math element; it builds a
// [value.MathUnderline].
var Underline = value.NewElement[*value.MathUnderline](value.Function{
	Name:       "math.underline",
	Positional: []value.Param{{Name: "body", Type: mathContentType}},
	F:          underlineImpl,
})

func underlineImpl(_ *value.FunctionCallContext, args []value.Value, _ value.NamedArgsWithDefaults) (value.Value, error) {
	body, err := mathContent(args[0])
	if err != nil {
		return nil, value.ArgErrorPosf(0, "%s", err.Error())
	}
	return &value.MathUnderline{Body: body}, nil
}
