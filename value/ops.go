package value

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/woodsbury/decimal128"
	"znkr.io/writst/syntax"
	"znkr.io/writst/types"
)

var ErrValueTooLarge = errors.New("value is too large")

func UnaryOp(op syntax.UnaryOp, x Value) (Value, error) {
	fn, ok := unaryops[unaryopKey{op, x.Type()}]
	if !ok {
		opstr := "'" + op.String() + "'"
		if op == syntax.Pos {
			opstr = "unary " + opstr
		}
		return nil, fmt.Errorf("cannot apply %s to %s", opstr, x.Type())
	}
	return fn(x)
}

func BinaryOp(op syntax.BinaryOp, x, y Value) (Value, error) {
	xt, yt := x.Type(), y.Type()
	switch {
	case op == syntax.Add && xt == types.None:
		return y, nil
	case op == syntax.Add && yt == types.None:
		return x, nil
	case op == syntax.NotIn:
		v, err := BinaryOp(syntax.In, x, y)
		if err != nil {
			return nil, err
		}
		return !v.(Bool), nil
	case op == syntax.In && y.Type() == types.Array:
		v := slices.ContainsFunc(y.(*Array).Elems, func(v Value) bool {
			return x.Equal(v)
		})
		return Bool(v), nil
	case op == syntax.Add && xt == types.Ratio && yt == types.Length:
		x, y = y, x
	case op == syntax.Mul && canLeftMulFloatOrInt.Contains(xt) && floatOrInt.Contains(yt):
		x, y = y, x
	}

	x, y = promote(op, x, y)
	switch op {
	case syntax.Eq:
		return Bool(x.Equal(y)), nil
	case syntax.Neq:
		return Bool(!x.Equal(y)), nil
	case syntax.Lt:
		c, err := Compare(x, y)
		if err != nil {
			return nil, err
		}
		return Bool(c < 0), nil
	case syntax.Gt:
		c, err := Compare(x, y)
		if err != nil {
			return nil, err
		}
		return Bool(c > 0), nil
	case syntax.Leq:
		c, err := Compare(x, y)
		if err != nil {
			return nil, err
		}
		return Bool(c <= 0), nil
	case syntax.Geq:
		c, err := Compare(x, y)
		if err != nil {
			return nil, err
		}
		return Bool(c >= 0), nil
	}

	key := binopKey{op, x.Type(), y.Type()}
	fn, ok := binops[key]
	if !ok {
		var verb, connective string
		at, bt := xt, yt
		switch op {
		case syntax.Add:
			verb = "add"
			connective = "and"
		case syntax.Sub:
			verb = "subtract"
			connective = "from"
			at, bt = yt, xt // swap for error message
		case syntax.Mul:
			verb = "multiply"
			connective = "with"
		case syntax.Div:
			verb = "divide"
			connective = "by"
		default:
			verb = fmt.Sprintf("perform %s on", op)
			connective = "and"
		}
		return nil, fmt.Errorf("cannot %s %s %s %s", verb, at, connective, bt)
	}
	return fn(x, y)
}

func Compare(x, y Value) (int, error) {
	if x.Type() != y.Type() {
		return 0, fmt.Errorf("cannot compare %v and %v", x.Type(), y.Type())
	}
	fn, ok := cmpops[x.Type()]
	if !ok {
		return 0, fmt.Errorf("cannot compare %v and %v", x.Type(), y.Type())
	}
	return fn(x, y)
}

type unaryopKey struct {
	op  syntax.UnaryOp
	typ types.Type
}

var unaryops = map[unaryopKey]func(x Value) (Value, error){
	{syntax.Not, types.Bool}: func(x Value) (Value, error) {
		return !x.(Bool), nil
	},
	{syntax.Pos, types.Int}: func(x Value) (Value, error) {
		return x, nil
	},
	{syntax.Neg, types.Int}: func(x Value) (Value, error) {
		return -x.(Int), nil
	},
	{syntax.Pos, types.Float}: func(x Value) (Value, error) {
		return x, nil
	},
	{syntax.Neg, types.Float}: func(x Value) (Value, error) {
		return -x.(Float), nil
	},
	{syntax.Pos, types.Decimal}: func(x Value) (Value, error) {
		return x, nil
	},
	{syntax.Neg, types.Decimal}: func(x Value) (Value, error) {
		return Decimal(decimal128.Decimal(x.(Decimal)).Neg()), nil
	},
	{syntax.Pos, types.Length}: func(x Value) (Value, error) {
		return x, nil
	},
	{syntax.Neg, types.Length}: func(x Value) (Value, error) {
		l := x.(Length)
		return Length{Pt: -l.Pt, Em: -l.Em}, nil
	},
	{syntax.Neg, types.Ratio}: func(x Value) (Value, error) {
		return -x.(Ratio), nil
	},
	{syntax.Pos, types.Ratio}: func(x Value) (Value, error) {
		return x, nil
	},
	{syntax.Pos, types.Angle}: func(x Value) (Value, error) {
		return x, nil
	},
	{syntax.Neg, types.Angle}: func(x Value) (Value, error) {
		return -x.(Angle), nil
	},
	{syntax.Neg, types.Fraction}: func(x Value) (Value, error) {
		return -x.(Fraction), nil
	},
	{syntax.Pos, types.Fraction}: func(x Value) (Value, error) {
		return x, nil
	},
	{syntax.Neg, types.Relative}: func(x Value) (Value, error) {
		r := x.(Relative)
		return Relative{Ratio: -r.Ratio, Length: Length{Pt: -r.Length.Pt, Em: -r.Length.Em}}, nil
	},
	{syntax.Pos, types.Relative}: func(x Value) (Value, error) {
		return x, nil
	},
}

type binopKey struct {
	op        syntax.BinaryOp
	leftType  types.Type
	rightType types.Type
}

var binops = map[binopKey]func(x, y Value) (Value, error){
	// Boolean operations
	{syntax.And, types.Bool, types.Bool}: func(x, y Value) (Value, error) {
		return x.(Bool) && y.(Bool), nil
	},
	{syntax.Or, types.Bool, types.Bool}: func(x, y Value) (Value, error) {
		return x.(Bool) || y.(Bool), nil
	},

	// Int operations
	{syntax.Add, types.Int, types.Int}: func(x, y Value) (Value, error) {
		xi, yi := x.(Int), y.(Int)
		if yi > 0 {
			if xi > math.MaxInt64-yi {
				return Int(0), ErrValueTooLarge
			}
		} else {
			if xi < math.MinInt64-yi {
				return Int(0), ErrValueTooLarge
			}
		}
		return xi + yi, nil
	},
	{syntax.Sub, types.Int, types.Int}: func(x, y Value) (Value, error) {
		xi, yi := x.(Int), y.(Int)
		if yi > 0 {
			if xi < math.MinInt64+yi {
				return Int(0), ErrValueTooLarge
			}
		} else {
			if xi > math.MaxInt64+yi {
				return Int(0), ErrValueTooLarge
			}
		}
		return xi - yi, nil
	},
	{syntax.Mul, types.Int, types.Int}: func(x, y Value) (Value, error) {
		return x.(Int) * y.(Int), nil
	},
	{syntax.Div, types.Int, types.Int}: func(x, y Value) (Value, error) {
		xi, yi := x.(Int), y.(Int)
		r := Float(float64(xi) / float64(yi))
		if yi == 0 {
			return r, fmt.Errorf("cannot divide by zero")
		}
		return r, nil
	},

	// Float operations
	{syntax.Add, types.Float, types.Float}: func(x, y Value) (Value, error) {
		return x.(Float) + y.(Float), nil
	},
	{syntax.Sub, types.Float, types.Float}: func(x, y Value) (Value, error) {
		return x.(Float) - y.(Float), nil
	},
	{syntax.Mul, types.Float, types.Float}: func(x, y Value) (Value, error) {
		return x.(Float) * y.(Float), nil
	},
	{syntax.Div, types.Float, types.Float}: func(x, y Value) (Value, error) {
		xf, yf := float64(x.(Float)), float64(y.(Float))
		if yf == 0 {
			return Float(xf / yf), fmt.Errorf("cannot divide by zero")
		}
		return Float(xf / yf), nil
	},

	// Decimal operations
	{syntax.Add, types.Decimal, types.Decimal}: func(x, y Value) (Value, error) {
		return Decimal(decimal128.Decimal(x.(Decimal)).Add(decimal128.Decimal(y.(Decimal)))), nil
	},
	{syntax.Sub, types.Decimal, types.Decimal}: func(x, y Value) (Value, error) {
		return Decimal(decimal128.Decimal(x.(Decimal)).Sub(decimal128.Decimal(y.(Decimal)))), nil
	},
	{syntax.Mul, types.Decimal, types.Decimal}: func(x, y Value) (Value, error) {
		return Decimal(decimal128.Decimal(x.(Decimal)).Mul(decimal128.Decimal(y.(Decimal)))), nil
	},
	{syntax.Div, types.Decimal, types.Decimal}: func(x, y Value) (Value, error) {
		xd := decimal128.Decimal(x.(Decimal))
		yd := decimal128.Decimal(y.(Decimal))
		if yd.IsZero() {
			return Decimal(xd.Quo(yd)), fmt.Errorf("cannot divide by zero")
		}
		return Decimal(xd.Quo(yd)), nil
	},

	// Length operations
	{syntax.Add, types.Length, types.Length}: func(x, y Value) (Value, error) {
		a, b := x.(Length), y.(Length)
		return Length{Pt: a.Pt + b.Pt, Em: a.Em + b.Em}, nil
	},
	{syntax.Sub, types.Length, types.Length}: func(x, y Value) (Value, error) {
		a, b := x.(Length), y.(Length)
		return Length{Pt: a.Pt - b.Pt, Em: a.Em - b.Em}, nil
	},
	{syntax.Mul, types.Float, types.Length}: func(x, y Value) (Value, error) {
		f, l := float64(x.(Float)), y.(Length)
		// If only one of the units is present, we can just multiply that one, to handle Inf and
		// NaN correctly (Inf * 0 = NaN).
		if l.Pt != 0 && l.Em == 0 {
			return Length{Pt: f * l.Pt}, nil
		} else if l.Pt == 0 && l.Em != 0 {
			return Length{Em: f * l.Em}, nil
		} else {
			return Length{Pt: l.Pt * f, Em: l.Em * f}, nil
		}
	},
	{syntax.Div, types.Length, types.Float}: func(x, y Value) (Value, error) {
		l, f := x.(Length), float64(y.(Float))
		var r Length
		// If only one of the units is present, we can just multiply that one, to handle Inf, NaN
		// and Zero correctly (Inf * 0 = NaN).
		if l.Pt != 0 && l.Em == 0 {
			r = Length{Pt: l.Pt / f}
		} else if l.Pt == 0 && l.Em != 0 {
			r = Length{Em: l.Em / f}
		} else {
			r = Length{Pt: l.Pt / f, Em: l.Em / f}
		}
		if f == 0 {
			return r, fmt.Errorf("cannot divide by zero")
		}
		return r, nil
	},
	{syntax.Div, types.Length, types.Length}: func(x, y Value) (Value, error) {
		a, b := x.(Length), y.(Length)
		if b.Em == 0 && a.Em == 0 {
			return Float(a.Pt / b.Pt), nil
		}
		if b.Pt == 0 && a.Pt == 0 {
			return Float(a.Em / b.Em), nil
		}
		return nil, fmt.Errorf("cannot divide these two lengths")
	},
	{syntax.Div, types.Length, types.Relative}: func(x, y Value) (Value, error) {
		l, r := x.(Length), y.(Relative)
		if r.Ratio != 0 {
			return nil, fmt.Errorf("cannot divide length by relative length")
		}
		if r.Length.Em == 0 && l.Em == 0 {
			if r.Length.Pt == 0 {
				return Float(l.Pt / r.Length.Pt), fmt.Errorf("cannot divide by zero")
			}
			return Float(l.Pt / r.Length.Pt), nil
		}
		if r.Length.Pt == 0 && l.Pt == 0 {
			if r.Length.Em == 0 {
				return Float(l.Em / r.Length.Em), fmt.Errorf("cannot divide by zero")
			}
			return Float(l.Em / r.Length.Em), nil
		}
		return nil, fmt.Errorf("cannot divide these lengths")
	},

	// Relative operations
	{syntax.Add, types.Relative, types.Relative}: func(x, y Value) (Value, error) {
		a, b := x.(Relative), y.(Relative)
		return Relative{Ratio: a.Ratio + b.Ratio, Length: Length{Pt: a.Length.Pt + b.Length.Pt, Em: a.Length.Em + b.Length.Em}}, nil
	},
	{syntax.Sub, types.Relative, types.Relative}: func(x, y Value) (Value, error) {
		a, b := x.(Relative), y.(Relative)
		return Relative{Ratio: a.Ratio - b.Ratio, Length: Length{Pt: a.Length.Pt - b.Length.Pt, Em: a.Length.Em - b.Length.Em}}, nil
	},
	{syntax.Mul, types.Float, types.Relative}: func(x, y Value) (Value, error) {
		f, r := float64(x.(Float)), y.(Relative)
		return Relative{Ratio: Ratio(f * float64(r.Ratio)), Length: Length{Pt: f * r.Length.Pt, Em: f * r.Length.Em}}, nil
	},
	{syntax.Div, types.Relative, types.Float}: func(x, y Value) (Value, error) {
		r, f := x.(Relative), float64(y.(Float))
		return Relative{Ratio: Ratio(float64(r.Ratio) / f), Length: Length{Pt: r.Length.Pt / f, Em: r.Length.Em / f}}, nil
	},
	{syntax.Div, types.Relative, types.Relative}: func(x, y Value) (Value, error) {
		a, b := x.(Relative), y.(Relative)
		switch {
		case a.Length.Pt == 0 && a.Length.Em == 0 && b.Length.Pt == 0 && b.Length.Em == 0:
			if b.Ratio == 0 {
				return Float(float64(a.Ratio) / float64(b.Ratio)), fmt.Errorf("cannot divide by zero")
			}
			return Float(float64(a.Ratio) / float64(b.Ratio)), nil
		case a.Ratio == 0 && a.Length.Em == 0 && b.Ratio == 0 && b.Length.Em == 0:
			if b.Length.Pt == 0 {
				return Float(a.Length.Pt / b.Length.Pt), fmt.Errorf("cannot divide by zero")
			}
			return Float(a.Length.Pt / b.Length.Pt), nil
		case a.Ratio == 0 && a.Length.Pt == 0 && b.Ratio == 0 && b.Length.Pt == 0:
			if b.Length.Em == 0 {
				return Float(a.Length.Em / b.Length.Em), fmt.Errorf("cannot divide by zero")
			}
			return Float(a.Length.Em / b.Length.Em), nil
		default:
			return nil, fmt.Errorf("cannot divide these two relative lengths")
		}
	},
	{syntax.Div, types.Relative, types.Ratio}: func(x, y Value) (Value, error) {
		r, ratio := x.(Relative), y.(Ratio)
		if r.Length.Pt != 0 || r.Length.Em != 0 {
			return nil, fmt.Errorf("cannot divide relative length by ratio")
		}
		if ratio == 0 {
			return Float(float64(r.Ratio) / float64(ratio)), fmt.Errorf("cannot divide by zero")
		}
		return Float(float64(r.Ratio) / float64(ratio)), nil
	},
	{syntax.Div, types.Relative, types.Length}: func(x, y Value) (Value, error) {
		r, l := x.(Relative), y.(Length)
		if r.Ratio != 0 {
			return nil, fmt.Errorf("cannot divide relative length by length")
		}
		if l.Em == 0 && r.Length.Em == 0 {
			if l.Pt == 0 {
				return Float(r.Length.Pt / l.Pt), fmt.Errorf("cannot divide by zero")
			}
			return Float(r.Length.Pt / l.Pt), nil
		}
		if l.Pt == 0 && r.Length.Pt == 0 {
			if l.Em == 0 {
				return Float(r.Length.Em / l.Em), fmt.Errorf("cannot divide by zero")
			}
			return Float(r.Length.Em / l.Em), nil
		}
		return nil, fmt.Errorf("cannot divide these lengths")
	},

	// Ratio operations
	{syntax.Add, types.Ratio, types.Ratio}: func(x, y Value) (Value, error) {
		return x.(Ratio) + y.(Ratio), nil
	},
	{syntax.Sub, types.Ratio, types.Ratio}: func(x, y Value) (Value, error) {
		return x.(Ratio) - y.(Ratio), nil
	},
	{syntax.Mul, types.Ratio, types.Ratio}: func(x, y Value) (Value, error) {
		return x.(Ratio) * y.(Ratio), nil
	},
	{syntax.Div, types.Ratio, types.Ratio}: func(x, y Value) (Value, error) {
		xr, yr := x.(Ratio), y.(Ratio)
		if yr == 0 {
			return Float(float64(xr) / float64(yr)), fmt.Errorf("cannot divide by zero")
		}
		return Float(float64(xr) / float64(yr)), nil
	},
	{syntax.Div, types.Ratio, types.Float}: func(x, y Value) (Value, error) {
		return Ratio(float64(x.(Ratio)) / float64(y.(Float))), nil
	},
	{syntax.Div, types.Ratio, types.Relative}: func(x, y Value) (Value, error) {
		ratio, r := x.(Ratio), y.(Relative)
		if r.Length.Pt != 0 || r.Length.Em != 0 {
			return nil, fmt.Errorf("cannot divide ratio by relative length")
		}
		if r.Ratio == 0 {
			return Float(float64(ratio) / float64(r.Ratio)), fmt.Errorf("cannot divide by zero")
		}
		return Float(float64(ratio) / float64(r.Ratio)), nil
	},
	{syntax.Mul, types.Float, types.Ratio}: func(x, y Value) (Value, error) {
		return Ratio(float64(x.(Float)) * float64(y.(Ratio))), nil
	},

	// Angle operations
	{syntax.Add, types.Angle, types.Angle}: func(x, y Value) (Value, error) {
		return x.(Angle) + y.(Angle), nil
	},
	{syntax.Sub, types.Angle, types.Angle}: func(x, y Value) (Value, error) {
		return x.(Angle) - y.(Angle), nil
	},
	{syntax.Mul, types.Float, types.Angle}: func(x, y Value) (Value, error) {
		return Angle(float64(x.(Float)) * float64(y.(Angle))), nil
	},
	{syntax.Div, types.Angle, types.Float}: func(x, y Value) (Value, error) {
		return Angle(float64(x.(Angle)) / float64(y.(Float))), nil
	},
	{syntax.Div, types.Angle, types.Angle}: func(x, y Value) (Value, error) {
		xa, ya := x.(Angle), y.(Angle)
		if ya == 0 {
			return Float(float64(xa) / float64(ya)), fmt.Errorf("cannot divide by zero")
		}
		return Float(float64(xa) / float64(ya)), nil
	},

	// Fraction operations
	{syntax.Add, types.Fraction, types.Fraction}: func(x, y Value) (Value, error) {
		return x.(Fraction) + y.(Fraction), nil
	},
	{syntax.Sub, types.Fraction, types.Fraction}: func(x, y Value) (Value, error) {
		return x.(Fraction) - y.(Fraction), nil
	},
	{syntax.Mul, types.Fraction, types.Fraction}: func(x, y Value) (Value, error) {
		return x.(Fraction) * y.(Fraction), nil
	},
	{syntax.Mul, types.Float, types.Fraction}: func(x, y Value) (Value, error) {
		return Fraction(float64(x.(Float)) * float64(y.(Fraction))), nil
	},
	{syntax.Div, types.Fraction, types.Float}: func(x, y Value) (Value, error) {
		return Fraction(float64(x.(Fraction)) / float64(y.(Float))), nil
	},
	{syntax.Div, types.Fraction, types.Fraction}: func(x, y Value) (Value, error) {
		xf, yf := x.(Fraction), y.(Fraction)
		if yf == 0 {
			return Float(float64(xf) / float64(yf)), fmt.Errorf("cannot divide by zero")
		}
		return Float(float64(xf) / float64(yf)), nil
	},

	// Cross-type operations producing Relative
	{syntax.Add, types.Length, types.Ratio}: func(x, y Value) (Value, error) {
		return Relative{Ratio: y.(Ratio), Length: x.(Length)}, nil
	},
	{syntax.Sub, types.Length, types.Ratio}: func(x, y Value) (Value, error) {
		return Relative{Ratio: -y.(Ratio), Length: x.(Length)}, nil
	},
	{syntax.Sub, types.Ratio, types.Length}: func(x, y Value) (Value, error) {
		l := y.(Length)
		return Relative{Ratio: x.(Ratio), Length: Length{Pt: -l.Pt, Em: -l.Em}}, nil
	},

	// String operations
	{syntax.Add, types.Str, types.Str}: func(x, y Value) (Value, error) {
		return Str(string(x.(Str)) + string(y.(Str))), nil
	},
	{syntax.Mul, types.Int, types.Str}: func(x, y Value) (Value, error) {
		times, str := x.(Int), y.(Str)
		if times < 0 {
			return nil, fmt.Errorf("number must be at least zero")
		}
		return Str(strings.Repeat(string(str), int(times))), nil
	},

	{syntax.In, types.Str, types.Str}: func(x, y Value) (Value, error) {
		return Bool(strings.Contains(string(y.(Str)), string(x.(Str)))), nil
	},

	// Bytes operations
	{syntax.Add, types.Bytes, types.Bytes}: func(x, y Value) (Value, error) {
		return Bytes(string(x.(Bytes)) + string(y.(Bytes))), nil
	},

	// Array operations
	{syntax.Add, types.Array, types.Array}: func(x, y Value) (Value, error) {
		arr1, arr2 := x.(*Array), y.(*Array)
		return &Array{Elems: slices.Concat(arr1.Elems, arr2.Elems)}, nil
	},
	{syntax.Mul, types.Array, types.Int}: func(x, y Value) (Value, error) {
		arr, times := x.(*Array), y.(Int)
		if times < 0 {
			return nil, fmt.Errorf("cannot multiply array by negative integer")
		}
		result := slices.Repeat(arr.Elems, int(times))
		return &Array{result}, nil
	},

	// Dictionary operations
	{syntax.In, types.Str, types.Dict}: func(x, y Value) (Value, error) {
		dict := y.(*Dict)
		_, ok := dict.Elems.Get(x.(Str))
		return Bool(ok), nil
	},
	{syntax.Add, types.Dict, types.Dict}: func(x, y Value) (Value, error) {
		d1, d2 := x.(*Dict), y.(*Dict)
		r := new(Dict)
		for k, v := range d1.Elems.All() {
			r.Elems.Put(k, v)
		}
		for k, v := range d2.Elems.All() {
			r.Elems.Put(k, v)
		}
		return &Dict{Elems: r.Elems}, nil
	},

	// Arguments operations
	{syntax.Add, types.Arguments, types.Arguments}: func(x, y Value) (Value, error) {
		a, b := x.(*Arguments), y.(*Arguments)
		return a.Merge(b), nil
	},

	// Content operations
	{syntax.Add, types.Content, types.Content}: func(x, y Value) (Value, error) {
		c1, c2 := x.(Content), y.(Content)
		return &Sequence{Children: []Content{c1, c2}}, nil
	},
	{syntax.Add, types.Content, types.Str}: func(x, y Value) (Value, error) {
		c, s := x.(Content), y.(Str)
		return &Sequence{Children: []Content{c, &Text{Text: string(s)}}}, nil
	},
	{syntax.Mul, types.Content, types.Int}: func(x, y Value) (Value, error) {
		c, times := x.(Content), y.(Int)
		if times < 0 {
			return nil, fmt.Errorf("cannot multiply content by negative integer")
		}
		result := slices.Repeat([]Content{c}, int(times))
		return &Sequence{Children: result}, nil
	},
}

var cmpops = map[types.Type]func(x, y Value) (int, error){
	types.Bool: func(x, y Value) (int, error) {
		if x.(Bool) == y.(Bool) {
			return 0, nil
		} else if !x.(Bool) && y.(Bool) {
			return -1, nil
		} else {
			return 1, nil
		}
	},
	types.Int: func(x, y Value) (int, error) { return cmp.Compare(x.(Int), y.(Int)), nil },
	types.Float: func(x, y Value) (int, error) {
		xf, yf := float64(x.(Float)), float64(y.(Float))
		if math.IsNaN(xf) || math.IsNaN(yf) {
			return 0, fmt.Errorf("cannot compare %s with %s", x, y)
		}
		return cmp.Compare(xf, yf), nil
	},
	types.Decimal: func(x, y Value) (int, error) {
		xd, yd := decimal128.Decimal(x.(Decimal)), decimal128.Decimal(y.(Decimal))
		return int(xd.Cmp(yd)), nil
	},
	types.Length: func(x, y Value) (int, error) {
		a, b := x.(Length), y.(Length)
		switch {
		case a.Pt != 0 && a.Em == 0 && b.Pt != 0 && b.Em == 0:
			return cmp.Compare(a.Pt, b.Pt), nil
		case a.Pt == 0 && a.Em != 0 && b.Pt == 0 && b.Em != 0:
			return cmp.Compare(a.Em, b.Em), nil
		default:
			return 0, fmt.Errorf("cannot compare %s with %s", a, b)
		}
	},
	types.Relative: func(x, y Value) (int, error) {
		a, b := x.(Relative), y.(Relative)
		switch {
		case a.Length.Pt == 0 && a.Length.Em == 0 && b.Length.Pt == 0 && b.Length.Em == 0:
			return cmp.Compare(a.Ratio, b.Ratio), nil
		case a.Ratio == 0 && a.Length.Em == 0 && b.Ratio == 0 && b.Length.Em == 0:
			return cmp.Compare(a.Length.Pt, b.Length.Pt), nil
		case a.Ratio == 0 && a.Length.Pt == 0 && b.Ratio == 0 && b.Length.Pt == 0:
			return cmp.Compare(a.Length.Em, b.Length.Em), nil
		default:
			return 0, fmt.Errorf("cannot compare %s with %s", a, b)
		}
	},
	types.Ratio:    func(x, y Value) (int, error) { return cmp.Compare(x.(Ratio), y.(Ratio)), nil },
	types.Angle:    func(x, y Value) (int, error) { return cmp.Compare(x.(Angle), y.(Angle)), nil },
	types.Fraction: func(x, y Value) (int, error) { return cmp.Compare(x.(Fraction), y.(Fraction)), nil },
	types.Str:      func(x, y Value) (int, error) { return cmp.Compare(x.(Str), y.(Str)), nil },
	types.Bytes:    func(x, y Value) (int, error) { return cmp.Compare(x.(Bytes), y.(Bytes)), nil },
}

func init() {
	cmpops[types.Array] = func(x, y Value) (int, error) {
		a, b := x.(*Array).Elems, y.(*Array).Elems
		n := min(len(a), len(b))
		for i := range n {
			ai, bi := promote(syntax.Eq, a[i], b[i])
			c, err := Compare(ai, bi)
			if err != nil {
				return 0, err
			}
			if c != 0 {
				return c, nil
			}
		}
		return cmp.Compare(len(a), len(b)), nil
	}
}

func promotionOrd(typ types.Type) int {
	switch typ {
	default:
		return -1
	case types.Bool:
		return 1
	case types.Int:
		return 2
	case types.Float:
		return 3
	case types.Length, types.Ratio:
		return 4
	case types.Relative:
		return 5
	case types.Decimal:
		return 6
	}
}

var promotesOtherArgFromIntToFloat = types.SetOf(
	types.Float,
	types.Length,
	types.Relative,
	types.Ratio,
	types.Relative,
	types.Angle,
	types.Fraction,
)

// promote0 must only be called by promote.
// Precondition: promotionOrd(v.Type()) < promotionOrd(target)
func promote0(op syntax.BinaryOp, v Value, target types.Type) Value {
	switch vt := v.Type(); {
	case vt == target:
		return v
	case vt == types.Int && promotesOtherArgFromIntToFloat.Contains(target):
		return Float(v.(Int))
	case op != syntax.Div && vt == types.Length && target == types.Relative:
		l := v.(Length)
		return Relative{Length: l}
	case op != syntax.Div && vt == types.Ratio && target == types.Relative:
		return Relative{Ratio: v.(Ratio)}
	case vt == types.Int && target == types.Decimal:
		return Decimal(decimal128.FromInt64(int64(v.(Int))))
	case vt == types.Float && target == types.Decimal:
		return Decimal(decimal128.FromFloat64(float64(v.(Float))))
	default:
		return v
	}
}

// promote promotes x and y to the same type if possible, following the promotion rules.
func promote(op syntax.BinaryOp, x, y Value) (Value, Value) {
	xt, yt := x.Type(), y.Type()
	// Special case: Int paired with unit types promotes Int to Float.
	switch {
	case xt == types.Int && promotesOtherArgFromIntToFloat.Contains(yt):
		x = Float(x.(Int))
	case yt == types.Int && promotesOtherArgFromIntToFloat.Contains(xt):
		y = Float(y.(Int))
	case promotionOrd(xt) < promotionOrd(yt):
		x = promote0(op, x, yt)
	case promotionOrd(xt) > promotionOrd(yt):
		y = promote0(op, y, xt)
	}
	return x, y
}

var canLeftMulFloatOrInt = types.SetOf(
	types.Length,
	types.Relative,
	types.Ratio,
	types.Fraction,
	types.Angle,
)

var floatOrInt = types.SetOf(
	types.Float,
	types.Int,
)
