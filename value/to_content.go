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
	case None:
		return nil, nil
	case Int:
		return &Raw{Lines: []string{fmt.Sprintf("%d", v)}}, nil
	case Float:
		s := v.String()
		if s == "float.nan" {
			s = "nan"
		}
		return &Raw{Lines: []string{s}}, nil
	case Length:
		return &Raw{Lines: []string{v.String()}}, nil
	case Relative:
		return &Raw{Lines: []string{v.String()}}, nil
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
		return &Raw{Lines: []string{s}}, nil
	case *Array:
		return &Raw{Lines: []string{FormatValue(v)}}, nil
	case *Dict:
		return &Raw{Lines: []string{FormatValue(v)}}, nil
	default:
		return nil, fmt.Errorf("content expression evaluated to non-content value: %T", v)
	}
}
