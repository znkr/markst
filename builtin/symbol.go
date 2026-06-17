package builtin

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"

	"znkr.io/writst/internal/graphemes"
	"znkr.io/writst/internal/symbols"
	"znkr.io/writst/name"
	"znkr.io/writst/types"
	"znkr.io/writst/value"
)

// Symbol is the symbol() constructor. It builds a custom symbol from one or
// more variants, each either a single-grapheme string (the default variant,
// with no modifiers) or a (modifiers, value) pair where modifiers is a
// dot-separated list of identifiers.
var Symbol = &value.Function{
	Name: "symbol",
	Positional: []value.Param{
		{Name: "variants", Type: types.Any},
	},
	Sink: new(0),
	F:    symbolImpl,
}

var Sym = &value.Module{
	Name: "sym",
	Def:  symbolsModDef(symbols.Symbols),
}

var Emoji = &value.Module{
	Name: "emoji",
	Def:  symbolsModDef(symbols.Emoji),
}

type symbolsModDef symbols.Module

func (m symbolsModDef) Get(name name.Name) value.Value {
	mm := symbols.Module(m)
	b, ok := mm.Get(name)
	if !ok {
		return nil
	}

	switch b := b.(type) {
	case symbols.Module:
		return &value.Module{
			Name: name.String(),
			Def:  symbolsModDef(b),
		}
	case symbols.Variants:
		return &value.Symbol{Variants: b}
	default:
		panic(fmt.Sprintf("unexpected binding type: %T", b))
	}
}

func symbolImpl(_ *value.FunctionCallContext, args []value.Value, _ value.NamedArgsWithDefaults) (value.Value, error) {
	variants := args[0].(*value.Arguments)
	if len(variants.Positional) == 0 {
		return nil, fmt.Errorf("expected at least one variant")
	}

	var errs []error
	var result symbols.Variants
	hasDefault := false           // a no-modifier (default) variant has been seen
	seen := map[string][]string{} // canonical (sorted) key -> first occurrence's mods in given order

	for i, v := range variants.Positional {
		modStr, val, err := splitVariant(i, v)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		mods, err := parseModifiers(i, modStr)
		if err != nil {
			errs = append(errs, err)
			continue
		}

		if !isSingleGrapheme(val) {
			err := value.ArgErrorPosf(i, "invalid variant value: %q", val)
			err.Hint("variant value must be exactly one grapheme cluster")
			errs = append(errs, err)
			continue
		}

		if len(mods) == 0 {
			if hasDefault {
				errs = append(errs, value.ArgErrorPosf(i, "duplicate default variant"))
				continue
			}
			hasDefault = true
		} else {
			key := canonicalKey(mods)
			if prev, ok := seen[key]; ok {
				err := value.ArgErrorPosf(i, "duplicate variant: %q", modStr)
				if !slices.Equal(prev, mods) {
					err.Hint("variants with the same modifiers are identical, regardless of their order")
				}
				errs = append(errs, err)
				continue
			}
			seen[key] = mods
		}

		handles := make([]name.Name, len(mods))
		for j, m := range mods {
			handles[j] = name.Make(m)
		}
		result = append(result, symbols.Symbol{Mods: handles, Value: val})
	}

	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return &value.Symbol{Variants: result}, nil
}

// splitVariant extracts the modifier string and value from a variant argument,
// which is either a single value (the default variant) or a (modifiers, value)
// pair. A variant value is either a single-grapheme string or a symbol, in
// which case its default variant's value is used.
func splitVariant(i int, v value.Value) (modStr, val string, err error) {
	switch v := v.(type) {
	case value.Str, *value.Symbol:
		s, err := variantValue(i, v)
		return "", s, err
	case *value.Array:
		if len(v.Elems) != 2 {
			return "", "", value.ArgErrorPosf(i, "expected array of exactly two strings, found array of length %d", len(v.Elems))
		}
		m, ok := v.Elems[0].(value.Str)
		if !ok {
			return "", "", value.ArgErrorPosf(i, "expected string, found %s", v.Elems[0].Type())
		}
		s, err := variantValue(i, v.Elems[1])
		if err != nil {
			return "", "", err
		}
		return string(m), s, nil
	default:
		return "", "", value.ArgErrorPosf(i, "expected string or array, found %s", v.Type())
	}
}

// variantValue extracts the string value of a variant from either a string or
// a symbol (using the symbol's default variant value).
func variantValue(i int, v value.Value) (string, error) {
	switch v := v.(type) {
	case value.Str:
		return string(v), nil
	case *value.Symbol:
		return v.String(), nil
	default:
		return "", value.ArgErrorPosf(i, "expected string or symbol, found %s", v.Type())
	}
}

// parseModifiers splits a dot-separated modifier string into its components,
// validating that each is a non-empty identifier and that none repeats. An
// empty string denotes the default variant and yields no modifiers.
func parseModifiers(i int, modStr string) ([]string, error) {
	if modStr == "" {
		return nil, nil
	}
	mods := strings.Split(modStr, ".")
	seen := map[string]bool{}
	for _, m := range mods {
		if !isValidModifier(m) {
			return nil, value.ArgErrorPosf(i, "invalid symbol modifier: %q", m)
		}
		if seen[m] {
			err := value.ArgErrorPosf(i, "duplicate modifier within variant: %q", m)
			err.Hint("modifiers are not ordered, so each one may appear only once")
			return nil, err
		}
		seen[m] = true
	}
	return mods, nil
}

// canonicalKey returns an order-independent key for a set of modifiers, so that
// variants with the same modifiers in a different order compare as equal.
func canonicalKey(mods []string) string {
	sorted := slices.Clone(mods)
	slices.Sort(sorted)
	return strings.Join(sorted, ".")
}

func isValidModifier(m string) bool {
	for i, r := range m {
		if i == 0 {
			if !unicode.IsLetter(r) && r != '_' {
				return false
			}
			continue
		}
		if !unicode.IsLetter(r) && !unicode.IsNumber(r) && r != '-' && r != '_' {
			return false
		}
	}
	return m != ""
}

// isSingleGrapheme reports whether s is exactly one grapheme cluster.
func isSingleGrapheme(s string) bool {
	if s == "" {
		return false
	}
	_, w := graphemes.Decode(s)
	return w == len(s)
}
