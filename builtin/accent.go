package builtin

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"znkr.io/writst/internal/names"
	"znkr.io/writst/types"
	"znkr.io/writst/value"
)

// textPresentation is the variation selector requesting a text (rather than
// emoji) rendering. It is decoration, not content.
const textPresentation = "︎"

// combiningAccents maps the characters that denote an accent to the combining
// mark they stand for. It covers both the standalone glyphs the symbol module
// produces (`hat` is `^`, `tilde` is `∼`, …) and the combining marks
// themselves, so that `hat(x)` and `accent(x, \u{0302})` agree.
//
// Membership here is also what makes a symbol callable: `hat(f)` works because
// `^` is listed; `pi(x)` is an error because `π` is not.
var combiningAccents = map[rune]rune{
	'̀': '̀', '`': '̀', // grave
	'́': '́', '´': '́', // acute
	'̂': '̂', '^': '̂', 'ˆ': '̂', // hat
	'̃': '̃', '~': '̃', '∼': '̃', '˜': '̃', // tilde
	'̄': '̄', '¯': '̄', // macron
	'̅': '̅', '-': '̅', '–': '̅', '−': '̅', '‾': '̅', // dash
	'̆': '̆', '˘': '̆', // breve
	'̇': '̇', '.': '̇', '⋅': '̇', '˙': '̇', // dot
	'̈': '̈', '¨': '̈', // diaer, dot.double
	'̉': '̉',                                         // hook above
	'̊': '̊', '∘': '̊', '○': '̊', '˚': '̊', '°': '̊', // circle
	'̋': '̋', '˝': '̋', // acute.double
	'̌': '̌', 'ˇ': '̌', // caron
	'⃛': '⃛',           // dot.triple
	'⃜': '⃜',           // dot.quad
	'⃐': '⃐', '↼': '⃐', // harpoon.lt
	'⃑': '⃑', '⇀': '⃑', // harpoon
	'⃖': '⃖', '←': '⃖', // arrow.l
	'⃗': '⃗', '→': '⃗', '⟶': '⃗', // arrow
	'⃡': '⃡', '↔': '⃡', '⟷': '⃡', // arrow.l.r
}

// combiningAccent maps an accent character to its combining form. ok is false
// for characters that don't denote an accent.
func combiningAccent(c rune) (rune, bool) {
	a, ok := combiningAccents[c]
	return a, ok
}

// singleRune returns the sole character of s, ignoring a trailing text
// presentation selector so that `sym.arrow.l.r` ("↔︎") reads as `↔`. An emoji
// presentation selector is left in place, so `emoji.arrow.l.r` stays a
// multi-codepoint string and is rejected by the callers.
//
// A decoding failure (an empty string, or one that isn't valid UTF-8) is not a
// single rune: utf8.DecodeRuneInString reports those as RuneError with a size
// below two, which a literal U+FFFD — three bytes — never hits.
func singleRune(s string) (rune, bool) {
	s = strings.TrimSuffix(s, textPresentation)
	c, size := utf8.DecodeRuneInString(s)
	if size != len(s) || (c == utf8.RuneError && size < 2) {
		return 0, false
	}
	return c, true
}

// accentRune extracts the character an accent argument denotes, normalizing a
// known accent to its combining form. Unknown characters pass through, so an
// explicit `accent(x, \u{0323})` may use an arbitrary combining mark.
//
// The error wording follows Typst and depends on how the accent was written: a
// string or symbol must be exactly one character, whereas content must be a
// single codepoint.
func accentRune(v value.Value) (rune, error) {
	var s string
	tooLong := "expected exactly one character"
	switch x := v.(type) {
	case value.Str:
		s = string(x)
	case *value.Symbol:
		s = x.String()
	case *value.MathText:
		s, tooLong = x.Text, "expected a single-codepoint symbol"
	case *value.Text:
		s, tooLong = x.Text, "expected a single-codepoint symbol"
	default:
		return 0, fmt.Errorf("expected a single-codepoint symbol")
	}
	c, ok := singleRune(s)
	if !ok {
		return 0, fmt.Errorf("%s", tooLong)
	}
	if a, ok := combiningAccent(c); ok {
		c = a
	}
	return c, nil
}

// SymbolFunc returns the function a symbol denotes when it is called. Accent
// symbols apply themselves — `hat(f)` is `accent(f, hat)` — and every other
// symbol is not callable.
func SymbolFunc(s *value.Symbol) (*value.Function, error) {
	c, ok := singleRune(s.String())
	if !ok {
		return nil, fmt.Errorf("symbol %s is not callable", s.String())
	}
	accent, ok := combiningAccent(c)
	if !ok {
		return nil, fmt.Errorf("symbol %s is not callable", s.String())
	}
	return &value.Function{
		Name:       "math.accent",
		Positional: []value.Param{{Name: "base", Type: mathContentType}},
		Named:      accentNamedParams(),
		F: func(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
			return newAccent(args[0], accent, named)
		},
	}, nil
}

// accentNamedParams are the properties shared by `math.accent` and the accent
// function a symbol denotes.
func accentNamedParams() value.NamedParams {
	return value.NamedParams{
		names.Size:    {Name: "size", Type: types.SetOf(types.Auto, types.Ratio, types.Length, types.Relative)},
		names.Dotless: {Name: "dotless", Type: types.SetOf(types.Bool), Default: value.Bool(true)},
	}
}

// newAccent builds the node shared by `math.accent` and the accent functions
// symbols denote.
func newAccent(base value.Value, accent rune, named value.NamedArgsWithDefaults) (value.Value, error) {
	b, err := mathContent(base)
	if err != nil {
		return nil, value.ArgErrorPosf(0, "%s", err.Error())
	}
	a := &value.MathAccent{
		Base:    b,
		Accent:  string(accent),
		Dotless: bool(named.Get(names.Dotless).(value.Bool)),
	}
	if named.IsSet(names.Size) {
		a.Size = named.Get(names.Size)
	}
	return a, nil
}

// Accent is the `accent(base, accent)` math element; it builds a
// [value.MathAccent].
var Accent = value.NewElement[*value.MathAccent](value.Function{
	Name: "math.accent",
	Positional: []value.Param{
		{Name: "base", Type: mathContentType},
		{Name: "accent", Type: mathContentType},
	},
	Named: accentNamedParams(),
	F:     accentImpl,
})

func accentImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	accent, err := accentRune(args[1])
	if err != nil {
		return nil, value.ArgErrorPosf(1, "%s", err.Error())
	}
	return newAccent(args[0], accent, named)
}
