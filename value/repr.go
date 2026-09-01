package value

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"znkr.io/markst/syntax"
)

// Repr renders v the way the `repr` function does: markst source that would
// produce the same value, as far as one exists. It is not the golden-file dump
// format — see [FormatValue] for that.
func Repr(v Value) string {
	switch v := v.(type) {
	case None:
		return "none"
	case Auto:
		return "auto"
	case Bool:
		return v.String()
	case Int:
		return v.String()
	case Float:
		return reprFloat(v)
	case Length:
		return v.String()
	case Angle:
		return v.String()
	case Relative:
		return v.String()
	case Ratio:
		return v.String()
	case Fraction:
		return v.String()
	case Str:
		return fmt.Sprintf("%q", string(v))
	case *Symbol:
		return reprSymbol(v)
	case Decimal:
		return fmt.Sprintf("decimal(%q)", v.String())
	case Datetime:
		return v.String()
	case Duration:
		return v.String()
	case *Label:
		return "<" + v.Name.String() + ">"
	case *Type:
		return v.Reflected.String()
	case *Module:
		return "<module " + v.Name + ">"
	case *Function:
		if v.Name == "" {
			return "(..) => .."
		}
		return v.Name
	case *Element:
		return v.Name
	case *Array:
		return reprArray(v)
	case *Dict:
		return reprDict(v)
	case *Arguments:
		return reprArguments(v)
	case Content:
		return reprContent(v)
	}
	return v.Type().String()
}

func reprFloat(v Float) string {
	switch {
	case math.IsInf(float64(v), 1):
		return "float.inf"
	case math.IsInf(float64(v), -1):
		return "-float.inf"
	case math.IsNaN(float64(v)):
		return "float.nan"
	}
	s := strconv.FormatFloat(float64(v), 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}

func reprSymbol(v *Symbol) string {
	var sb strings.Builder
	sb.WriteString("symbol(")
	multiline := len(v.Variants) > 2
	if multiline {
		sb.WriteString("\n")
	}
	for i, variant := range v.Variants {
		if multiline {
			sb.WriteString("  ")
		}
		if len(variant.Mods) == 0 {
			fmt.Fprintf(&sb, "%q", variant.Value)
		} else {
			mods := make([]string, len(variant.Mods))
			for i, mod := range variant.Mods {
				mods[i] = mod.String()
			}
			fmt.Fprintf(&sb, "(%q, %q)", strings.Join(mods, "."), variant.Value)
		}
		switch {
		case multiline:
			sb.WriteString(",\n")
		case i < len(v.Variants)-1:
			sb.WriteString(", ")
		}
	}
	sb.WriteString(")")
	return sb.String()
}

// maxReprItems is how many elements of an array or dictionary are spelled out
// before the rest are summarized.
const maxReprItems = 40

func reprArray(v *Array) string {
	parts := make([]string, 0, len(v.Elems))
	for _, e := range v.Elems[:min(len(v.Elems), maxReprItems)] {
		parts = append(parts, Repr(e))
	}
	if n := len(v.Elems) - maxReprItems; n > 0 {
		parts = append(parts, fmt.Sprintf(".. (%d items omitted)", n))
	}
	// A single element needs the trailing comma to stay an array rather than a
	// parenthesized expression.
	return prettyArrayLike(parts, len(v.Elems) == 1)
}

func reprDict(v *Dict) string {
	if v.Elems.Len() == 0 {
		return "(:)"
	}
	var parts []string
	for k, val := range v.Elems.All() {
		if len(parts) == maxReprItems {
			parts = append(parts, fmt.Sprintf(".. (%d pairs omitted)", v.Elems.Len()-maxReprItems))
			break
		}
		key := string(k)
		if !syntax.IsIdent(key) {
			key = strconv.Quote(key)
		}
		parts = append(parts, key+": "+Repr(val))
	}
	return prettyArrayLike(parts, false)
}

func reprArguments(v *Arguments) string {
	var parts []string
	for _, e := range v.Positional {
		parts = append(parts, Repr(e))
	}
	for k, val := range v.Named.All() {
		parts = append(parts, k.String()+": "+Repr(val))
	}
	return "arguments" + prettyArrayLike(parts, false)
}

// reprContent renders content as the call that builds it: the element name
// followed by the fields that are set. Text is spelled as the content block
// `[…]` it is written as, and a sequence as `sequence(…)`; neither is
// expressible through [Content.Fields].
func reprContent(c Content) string {
	switch c := c.(type) {
	case *Text:
		return "[" + c.Text + "]"
	case *MathText:
		return "[" + c.Text + "]"
	case *Sequence:
		if len(c.Children) == 0 {
			return "[]"
		}
		parts := make([]string, len(c.Children))
		for i, child := range c.Children {
			parts[i] = Repr(child)
		}
		return "sequence" + prettyArrayLike(parts, false)
	}
	var parts []string
	for k, v := range c.Fields().Elems.All() {
		parts = append(parts, string(k)+": "+Repr(v))
	}
	return c.Name() + prettyArrayLike(parts, false)
}

// maxReprWidth is the width at which a repr list breaks onto several lines.
const maxReprWidth = 50

// prettyCommaList joins parts with `, `, switching to one part per line once
// the single-line form would be wider than [maxReprWidth].
func prettyCommaList(parts []string, trailingComma bool) string {
	width := 2 * max(len(parts)-1, 0)
	for _, p := range parts {
		width += len(p)
	}
	if width <= maxReprWidth {
		s := strings.Join(parts, ", ")
		if trailingComma {
			s += ","
		}
		return s
	}
	var sb strings.Builder
	for _, p := range parts {
		sb.WriteString(strings.TrimSpace(p))
		sb.WriteString(",\n")
	}
	return sb.String()
}

// prettyArrayLike wraps [prettyCommaList] in parentheses, indenting every line
// by two when the list runs over several of them.
func prettyArrayLike(parts []string, trailingComma bool) string {
	list := prettyCommaList(parts, trailingComma)
	if !strings.Contains(list, "\n") {
		return "(" + list + ")"
	}
	var sb strings.Builder
	sb.WriteString("(\n")
	for line := range strings.SplitSeq(strings.TrimSuffix(list, "\n"), "\n") {
		sb.WriteString("  ")
		sb.WriteString(line)
		sb.WriteString("\n")
	}
	sb.WriteString(")")
	return sb.String()
}
