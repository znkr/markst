package builtin

import (
	"unicode/utf8"

	"znkr.io/writst/internal/names"
	"znkr.io/writst/name"
	"znkr.io/writst/types"
	"znkr.io/writst/value"
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
		names.Root:      Root,
		names.Sqrt:      Sqrt,
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
	return Sym.Def.Get(n)
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
// [value.Text] / [value.Raw] that [value.ToContent] would produce, so that a
// letter and a symbol resolving to the same character agree — the same rule the
// evaluator applies to math operands (see frame.mathContentOf). The math
// elements are math-only whichever mode they are called from, so no context is
// needed to decide this.
//
// The result is never nil: none becomes empty content, matching frame.contentOf.
func mathContent(v value.Value) (value.Content, error) {
	c, err := value.ToMathContent(v)
	if err != nil {
		return nil, err
	}
	if c == nil {
		return &value.Sequence{}, nil
	}
	return c, nil
}

// mathCells converts a run of argument values into content cells, flattening
// any array values (rows produced by the analyzer for `;`-separated lists).
func mathCells(vals []value.Value) ([]value.Content, error) {
	var out []value.Content
	for i, v := range vals {
		if arr, ok := v.(*value.Array); ok {
			for _, e := range arr.Elems {
				c, err := mathContent(e)
				if err != nil {
					return nil, value.ArgErrorPosf(i, "%s", err.Error())
				}
				out = append(out, c)
			}
			continue
		}
		c, err := mathContent(v)
		if err != nil {
			return nil, value.ArgErrorPosf(i, "%s", err.Error())
		}
		out = append(out, c)
	}
	return out, nil
}

// delimType is the accepted type of the vec/mat/cases `delim` property: none,
// a string or symbol delimiter, or a (open, close) array pair.
var delimType = types.SetOf(types.None, types.Str, types.Symbol, types.Array)

// rejectSinkNamed reports the first named argument captured by an element's
// positional sink as an error. vec/mat/cases have a positional children/rows
// sink and a fixed set of named properties, so any named arg that reached the
// sink is unrecognized (bind routes recognized ones to the declared params).
func rejectSinkNamed(sink *value.Arguments) error {
	for n := range sink.Named.All() {
		return value.ArgErrorNamedPairf(n, "unexpected argument: %s", n)
	}
	return nil
}

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
	children, err := mathCells(sink.Positional)
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
	children, err := mathCells(sink.Positional)
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
			row, err := mathCells(arr.Elems)
			if err != nil {
				return nil, err
			}
			rows = append(rows, row)
		}
	} else if row, err := mathCells(pos); err != nil {
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
		// mathStyle). Typst marks these internal: they are declared so that `#set
		// math.equation(bold: true)` — which never runs equationImpl — can carry
		// them, but an equation has nowhere to keep a style of its own, so the
		// constructor rejects them (see equationImpl).
		names.Bold:    {Name: "bold", Type: types.SetOf(types.Bool)},
		names.Italic:  {Name: "italic", Type: types.SetOf(types.Bool)},
		names.Variant: {Name: "variant", Type: types.SetOf(types.Str)},
	},
	F: equationImpl,
})

func equationImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	// The font-style properties only mean something as a set rule, which the
	// realization pass folds into the math leaves it covers. Constructing an
	// equation with one would silently drop it, so it is an error instead.
	for _, field := range mathStyleFields {
		if named.IsSet(field) {
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

// Sqrt is the `sqrt(radicand)` math function; it builds an index-less
// [value.MathRoot]. It is a plain function, not an element: the content it
// produces *is* a `math.root`, so [Root] is the set/show target for it, exactly
// as in Typst. Making it an element too would give two elements the same
// content type, and [value.Element] matches on that type alone — `#show
// math.sqrt:` would then also rewrite `$root(3, x)$`.
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

// lrFunc builds a math function that wraps its single content argument in the
// given open/close delimiters (e.g. floor → ⌊ ⌋).
//
// These are functions rather than elements for the same reason as [Sqrt]: they
// all produce a [value.MathDelimited], i.e. a `math.lr`, and [value.Element]
// matches on the content type alone — as elements, `#show math.floor:` would
// also rewrite `$abs(x)$`, `$ceil(x)$` and `$norm(x)$`.
func lrFunc(name, open, closing string) *value.Function {
	return &value.Function{
		Name:       name,
		Positional: []value.Param{{Name: "body", Type: mathContentType}},
		F: func(_ *value.FunctionCallContext, args []value.Value, _ value.NamedArgsWithDefaults) (value.Value, error) {
			body, err := mathContent(args[0])
			if err != nil {
				return nil, value.ArgErrorPosf(0, "%s", err.Error())
			}
			return &value.MathDelimited{
				Open:  &value.MathText{Text: open},
				Body:  body,
				Close: &value.MathText{Text: closing},
			}, nil
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
// the cancelled body and the `angle` field (retained for field access).
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
	if named.IsSet(names.Angle) {
		c.Angle = named.Get(names.Angle)
	}
	return c, nil
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
