package value

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/woodsbury/decimal128"
	"znkr.io/markst/types"
)

// None is the unit value, written as `none` in Markst.
type None struct{}

// Auto is the automatic value, written as `auto` in Markst.
type Auto struct{}

// Bool is a boolean value.
type Bool bool

// Int is a 64-bit signed integer.
type Int int64

// Float is a 64-bit floating-point number.
type Float float64

// Decimal is a 128-bit IEEE 754 decimal floating-point number.
type Decimal decimal128.Decimal

// Str is a string value.
type Str string

// Bytes is a byte string value.
type Bytes string

// Ratio represents a ratio (percentage), stored as a fraction of 1
// (e.g. 0.5 for 50%).
type Ratio float64

// Fraction represents a fractional unit (e.g. 1fr, 2fr) used in layout.
type Fraction float64

// Length represents a physical length with absolute (pt) and font-relative
// (em) components that are combined at layout time.
type Length struct {
	Pt float64 // Points (absolute component)
	Em float64 // Em units (font-relative component)
}

// Relative is the combination of a [Ratio] and a [Length], representing a
// length that is partially proportional and partially absolute.
type Relative struct {
	Ratio  Ratio
	Length Length
}

// Angle represents an angle in radians.
type Angle float64

func (n None) String() string { return "none" }

func (n Auto) String() string { return "auto" }

func (n Bool) String() string { return fmt.Sprintf("%t", n) }

func (n Int) String() string { return strconv.FormatInt(int64(n), 10) }

// formatUnit renders a scalar and its unit, rounding to two decimals as Typst
// does for every unit-suffixed value. The rounding is what keeps binary
// floating point out of the output: 220% is held as 2.2, and 2.2*100 is
// 220.00000000000003.
func formatUnit(v float64, unit string) string {
	// Rounding a value that large has no effect, and the multiplication below
	// would overflow to infinity.
	if !math.IsInf(v, 0) && !math.IsNaN(v) && math.Abs(v) < 1<<53 {
		v = math.Round(v*100) / 100
	}
	return fmt.Sprintf("%g%s", v, unit)
}

func (n Ratio) String() string {
	return formatUnit(float64(n)*100, "%")
}

func (n Fraction) String() string {
	return formatUnit(float64(n), "fr")
}

func (n Float) String() string {
	v := float64(n)
	switch {
	case math.IsInf(v, 1):
		return "float.inf"
	case math.IsInf(v, -1):
		return "float.-inf"
	case math.IsNaN(v):
		return "float.nan"
	default:
		s := strconv.FormatFloat(float64(v), 'f', -1, 64)
		return s
	}
}

func (n Decimal) String() string {
	if decimal128.Decimal(n).IsInf(1) {
		return "decimal.inf"
	}
	if decimal128.Decimal(n).IsInf(-1) {
		return "decimal.-inf"
	}
	if decimal128.Decimal(n).IsNaN() {
		return "decimal.nan"
	}
	return decimal128.Format(decimal128.Decimal(n), 'f', -1)
}

func (n Length) String() string {
	if math.IsInf(n.Pt+n.Em, 1) {
		return "inf"
	}
	if math.IsInf(n.Pt+n.Em, -1) {
		return "-inf"
	}
	if math.IsNaN(n.Pt + n.Em) {
		return "nan"
	}
	var sb strings.Builder
	if n.Pt != 0 {
		sb.WriteString(formatUnit(n.Pt, "pt"))
	}
	if n.Pt != 0 && n.Em != 0 {
		sb.WriteString(" + ")
	}
	if n.Em != 0 {
		sb.WriteString(formatUnit(n.Em, "em"))
	}
	if sb.Len() == 0 {
		sb.WriteString("0pt")
	}
	return sb.String()
}

func (r Relative) String() string {
	return fmt.Sprintf("%s + %s", r.Ratio, r.Length)
}

func (n Angle) String() string {
	return formatUnit(float64(n*180/math.Pi), "deg")
}

func (None) aValue()     {}
func (Auto) aValue()     {}
func (Bool) aValue()     {}
func (Int) aValue()      {}
func (Float) aValue()    {}
func (Decimal) aValue()  {}
func (Str) aValue()      {}
func (Bytes) aValue()    {}
func (Ratio) aValue()    {}
func (Fraction) aValue() {}
func (Length) aValue()   {}
func (Relative) aValue() {}
func (Angle) aValue()    {}

func (None) Type() types.Type     { return types.None }
func (Auto) Type() types.Type     { return types.Auto }
func (Bool) Type() types.Type     { return types.Bool }
func (Int) Type() types.Type      { return types.Int }
func (Float) Type() types.Type    { return types.Float }
func (Decimal) Type() types.Type  { return types.Decimal }
func (Str) Type() types.Type      { return types.Str }
func (Bytes) Type() types.Type    { return types.Bytes }
func (Ratio) Type() types.Type    { return types.Ratio }
func (Fraction) Type() types.Type { return types.Fraction }
func (Length) Type() types.Type   { return types.Length }
func (Relative) Type() types.Type { return types.Relative }
func (Angle) Type() types.Type    { return types.Angle }
