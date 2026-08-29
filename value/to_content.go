package value

import (
	"fmt"

	"github.com/woodsbury/decimal128"
)

func ToContent(v Value) (Content, error) {
	switch v := v.(type) {
	case Content:
		return v, nil
	case Str:
		return &Text{Text: string(v)}, nil
	case *Symbol:
		return &Text{Text: v.String()}, nil
	case None:
		return nil, nil
	case Int:
		return &Raw{Text: fmt.Sprintf("%d", v)}, nil
	case Float:
		return &Raw{Text: floatText(v)}, nil
	case Length:
		return &Raw{Text: v.String()}, nil
	case Relative:
		return &Raw{Text: v.String()}, nil
	case Decimal:
		d := decimal128.Decimal(v)
		var s string
		if d.IsInf(1) {
			s = "inf"
		} else if d.IsInf(-1) {
			s = "-inf"
		} else if d.IsNaN() {
			s = "nan"
		} else {
			s = d.String()
		}
		return &Raw{Text: s}, nil
	case Datetime:
		return &Raw{Text: v.String()}, nil
	case Duration:
		return &Raw{Text: v.String()}, nil
	case *Array:
		return &Raw{Text: FormatValue(v)}, nil
	case *Dict:
		return &Raw{Text: FormatValue(v)}, nil
	case *Function:
		return &Raw{Text: v.Name}, nil
	default:
		return nil, fmt.Errorf("content expression evaluated to non-content value: %T", v)
	}
}

// ToMathContent is [ToContent] in math context: symbols, strings and numbers
// become [MathText] rather than the upright [Text] / [Raw] that ToContent
// produces, so that the same character written as a letter, as a symbol, or as
// an argument to a math element renders the same way — `$sym.alpha + 1$`,
// `$alpha + 1$` and `#math.frac(alpha, 1)` all agree. Every other value is
// left to ToContent.
func ToMathContent(v Value) (Content, error) {
	switch v := v.(type) {
	case *Symbol:
		return &MathText{Text: v.String()}, nil
	case Str:
		return &MathText{Text: string(v)}, nil
	case Int:
		return &MathText{Text: fmt.Sprintf("%d", v)}, nil
	case Float:
		return &MathText{Text: floatText(v)}, nil
	}
	return ToContent(v)
}

// floatText renders a float the way content displays it: NaN is spelled the way
// it is written in source rather than as the repr `float.nan`.
func floatText(v Float) string {
	if s := v.String(); s != "float.nan" {
		return s
	}
	return "nan"
}
