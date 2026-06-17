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
		s := v.String()
		if s == "float.nan" {
			s = "nan"
		}
		return &Raw{Text: s}, nil
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
