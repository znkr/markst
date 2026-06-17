package builtin

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"znkr.io/writst/internal/lorem"
	"znkr.io/writst/types"
	"znkr.io/writst/value"
)

var (
	Repr = &value.Function{
		Name:       "repr",
		Positional: []value.Param{{Type: types.Any}},
		F:          reprImpl,
	}

	Type = &value.Function{
		Name:       "type",
		Positional: []value.Param{{Type: types.Any}},
		// F is set in init() to avoid an initialization cycle.
	}

	Lorem = &value.Function{
		Name: "lorem",
		Positional: []value.Param{
			{Name: "words", Type: types.SetOf(types.Int)},
		},
		F: loremImpl,
	}
)

func init() {
	Type.F = typeImpl
}

func reprImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	switch v := args[0].(type) {
	case value.None:
		return value.Str("none"), nil
	case value.Bool:
		return value.Str(v.String()), nil
	case value.Int:
		return value.Str(v.String()), nil
	case value.Float:
		if math.IsInf(float64(v), 1) {
			return value.Str("float.inf"), nil
		}
		if math.IsInf(float64(v), -1) {
			return value.Str("-float.inf"), nil
		}
		if math.IsNaN(float64(v)) {
			return value.Str("float.nan"), nil
		}
		s := value.Str(strconv.FormatFloat(float64(v), 'f', -1, 64))
		if !strings.Contains(string(s), ".") {
			s += ".0"
		}
		return s, nil
	case value.Length:
		return value.Str(v.String()), nil
	case value.Angle:
		return value.Str(v.String()), nil
	case value.Relative:
		return value.Str(v.String()), nil
	case value.Ratio:
		return value.Str(v.String()), nil
	case value.Fraction:
		return value.Str(v.String()), nil
	case value.Str:
		return value.Str(fmt.Sprintf("%q", string(v))), nil
	case *value.Symbol:
		var sb strings.Builder
		sb.WriteString("symbol(")
		if len(v.Variants) > 2 {
			sb.WriteString("\n")
		}
		for i, variant := range v.Variants {
			if len(v.Variants) > 2 {
				sb.WriteString("  ")
			}
			if len(variant.Mods) == 0 {
				fmt.Fprintf(&sb, "\"%s\"", variant.Value)
			} else {
				sb.WriteString("(\"")
				for i, mod := range variant.Mods {
					if i > 0 {
						sb.WriteString(".")
					}
					sb.WriteString(mod.String())
				}
				sb.WriteString("\", \"")
				sb.WriteString(variant.Value)
				sb.WriteString("\")")
			}
			if len(v.Variants) > 2 {
				sb.WriteString(",\n")
			} else if i < len(v.Variants)-1 {
				sb.WriteString(", ")
			}
		}
		sb.WriteString(")")
		return value.Str(sb.String()), nil
	case value.Decimal:
		s := v.String()
		return value.Str(fmt.Sprintf("decimal(\"%s\")", s)), nil
	case *value.Array:
		elems := make([]string, len(v.Elems))
		for i, e := range v.Elems {
			r, err := reprImpl(nil, []value.Value{e}, named)
			if err != nil {
				return nil, err
			}
			elems[i] = string(r.(value.Str))
		}
		return value.Str("(" + strings.Join(elems, ", ") + ")"), nil
	case *value.Dict:
		var parts []string
		for k, val := range v.Elems.All() {
			r, err := reprImpl(nil, []value.Value{val}, named)
			if err != nil {
				return nil, err
			}
			parts = append(parts, fmt.Sprintf("%q: %s", string(k), string(r.(value.Str))))
		}
		return value.Str("(" + strings.Join(parts, ", ") + ")"), nil
	case *value.Arguments:
		var parts []string
		for _, e := range v.Positional {
			r, err := reprImpl(nil, []value.Value{e}, named)
			if err != nil {
				return nil, err
			}
			parts = append(parts, string(r.(value.Str)))
		}
		for k, val := range v.Named.All() {
			r, err := reprImpl(nil, []value.Value{val}, named)
			if err != nil {
				return nil, err
			}
			parts = append(parts, fmt.Sprintf("%s: %s", k, string(r.(value.Str))))
		}
		return value.Str("arguments(" + strings.Join(parts, ", ") + ")"), nil
	default:
		panic(fmt.Sprintf("repr() not implemented for type %s", args[0].Type()))
	}
}

func typeImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	return reflectedTypes[args[0].Type()], nil
}

func loremImpl(_ *value.FunctionCallContext, args []value.Value, named value.NamedArgsWithDefaults) (value.Value, error) {
	words := int(args[0].(value.Int))
	lg := lorem.NewGenerator()
	return value.Str(lg.Words(words)), nil
}
