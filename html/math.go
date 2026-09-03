package html

// Math is rendered as MathML, which the browser lays out. This file therefore
// translates rather than typesets: it picks the MathML element with the right
// meaning and leaves spacing and sizing to the browser's math rules.

import (
	"strconv"
	"strings"
	"unicode"

	"znkr.io/markst/value"
)

// renderEquation writes an equation as a <math> element. A block equation that
// contains linebreaks or alignment points becomes a table, which is the only
// way MathML has to line several rows up.
func (e *Encoder) renderEquation(c *value.Equation) {
	display := Attr{}
	if c.Block {
		display = Attr{"display", "block"}
	}
	e.Start("math", display, idAttr(c))

	block := e.mathBlock
	e.mathBlock = c.Block
	items := flattenMath(c.Body)
	if c.Block && hasMathRows(items) {
		e.renderMathTable(items)
	} else {
		e.renderMathRow(items)
	}
	e.mathBlock = block

	e.End("math")
	if c.Block {
		e.Newline()
	}
}

// renderMathTable lays items out as rows split at linebreaks and columns split
// at alignment points. Where there are alignment points the column alignment
// alternates, starting right, so that `x &= y` puts the = at the left of its
// column; rows with nothing to align against are centered, as the equation
// itself would be.
func (e *Encoder) renderMathTable(items []value.Content) {
	isLinebreak := func(c value.Content) bool { _, ok := c.(*value.Linebreak); return ok }
	isAlignPoint := func(c value.Content) bool { _, ok := c.(*value.MathAlignPoint); return ok }

	var rows [][][]value.Content
	aligned := false
	for _, row := range splitMath(items, isLinebreak) {
		cells := splitMath(row, isAlignPoint)
		aligned = aligned || len(cells) > 1
		rows = append(rows, cells)
	}

	align := "center"
	if aligned {
		align = "right left"
	}
	e.Start("mtable", Attr{"columnalign", align})
	for _, cells := range rows {
		e.Start("mtr")
		for _, cell := range cells {
			e.Start("mtd")
			e.renderMathRow(cell)
			e.End("mtd")
		}
		e.End("mtr")
	}
	e.End("mtable")
}

// renderMathRow writes items as one MathML row. The <mrow> is what makes a run
// of elements a single argument, so it is written whenever there is more than
// one element to hold.
func (e *Encoder) renderMathRow(items []value.Content) {
	if len(items) == 1 {
		e.renderMath(items[0])
		return
	}
	e.Start("mrow")
	for _, item := range items {
		e.renderMath(item)
	}
	e.End("mrow")
}

// renderMathGroup writes c as a single MathML element, as each slot of a
// script, fraction, or root requires.
func (e *Encoder) renderMathGroup(c value.Content) {
	e.renderMathRow(flattenMath(c))
}

func (e *Encoder) renderMath(c value.Content) {
	if e.err != nil || c == nil {
		return
	}
	switch c := c.(type) {
	case *value.Sequence:
		e.renderMathRow(flattenMath(c))
	case *value.Equation:
		// A nested equation is part of the same formula. MathML has one <math>
		// per formula, so a second would start another.
		e.renderMathRow(flattenMath(c.Body))
	case *value.MathText:
		e.renderMathText(c)
	case *value.MathOp:
		// An operator is <mo> in MathML, which prevents `sin x` from setting
		// as `sinx`. The spacing is given explicitly rather than left to the
		// operator dictionary, which has no entry for a word like "sin".
		e.Start("mo", Attr{"lspace", "0.1667em"}, Attr{"rspace", "0.1667em"})
		e.Text(mathTextContent(c.Text))
		e.End("mo")
	case *value.MathAttach:
		e.renderMathAttach(c)
	case *value.MathPrimes:
		e.Start("msup")
		e.renderMathGroup(c.Base)
		e.Start("mo")
		e.Text(primes(c.Count))
		e.End("mo")
		e.End("msup")
	case *value.MathFrac:
		e.Start("mfrac")
		e.renderMathGroup(c.Num)
		e.renderMathGroup(c.Denom)
		e.End("mfrac")
	case *value.MathRoot:
		if c.Index == nil {
			e.Start("msqrt")
			e.renderMathGroup(c.Radicand)
			e.End("msqrt")
			return
		}
		e.Start("mroot")
		e.renderMathGroup(c.Radicand)
		e.renderMathGroup(c.Index)
		e.End("mroot")
	case *value.MathLr:
		e.renderMathLr(c)
	case *value.MathMid:
		// A mid scales with its enclosing group, so it takes the same stretch
		// setting as that group's delimiters.
		e.renderMathDelim(c.Body, e.mathStretch)
	case *value.MathUnderline:
		e.Start("munder", Attr{"accentunder", "true"})
		e.renderMathGroup(c.Body)
		e.Start("mo", Attr{"stretchy", "true"})
		e.Text("̲")
		e.End("mo")
		e.End("munder")
	case *value.MathAccent:
		e.Start("mover", Attr{"accent", "true"})
		e.renderMathGroup(c.Base)
		e.Start("mo")
		e.Text(c.Accent)
		e.End("mo")
		e.End("mover")
	case *value.MathCancel:
		// MathML Core removed <menclose>, so the line is drawn with CSS. The
		// angle cannot be expressed either way and is dropped.
		e.Start("mrow", Attr{"style", "text-decoration:line-through"})
		e.renderMathGroup(c.Body)
		e.End("mrow")
	case *value.MathVec:
		e.renderMathMatrix("(", ")", "center", rowsOf(c.Children))
	case *value.MathCases:
		e.renderMathMatrix("{", "", "left", rowsOf(c.Children))
	case *value.MathMat:
		e.renderMathMatrix("(", ")", "center", c.Rows)
	case *value.MathAlignPoint:
		// Only a block equation lays its rows out in a table, where alignment
		// points become the column breaks. Elsewhere there is nothing to align
		// against.
		e.HTML("<mspace></mspace>")
	case *value.Linebreak:
		e.Start("mspace", Attr{"linebreak", "newline"})
		e.End("mspace")
	case *value.HSpace:
		e.Start("mspace", Attr{"width", cssLength(c.Amount)})
		e.End("mspace")
	case *value.Text:
		e.Start("mtext")
		e.Text(c.Text)
		e.End("mtext")
	case *value.Raw:
		e.Start("mtext")
		e.Text(c.Text)
		e.End("mtext")
	default:
		// Ordinary markup inside an equation: a link, an image, a custom
		// element. <mtext> is the MathML element for text and the phrasing
		// content around it.
		e.Start("mtext")
		e.render(c)
		e.End("mtext")
	}
}

// renderMathAttach writes a base with its sub- and superscripts. An operator
// that takes limits puts them above and below itself, but only in a block
// equation: inline, they would push the line apart.
func (e *Encoder) renderMathAttach(c *value.MathAttach) {
	limits := e.mathBlock && takesLimits(c.Base)
	var tag string
	switch {
	case c.Top != nil && c.Bottom != nil:
		tag = "msubsup"
		if limits {
			tag = "munderover"
		}
	case c.Top != nil:
		tag = "msup"
		if limits {
			tag = "mover"
		}
	case c.Bottom != nil:
		tag = "msub"
		if limits {
			tag = "munder"
		}
	default:
		e.renderMathGroup(c.Base)
		return
	}

	e.Start(tag)
	e.renderMathGroup(c.Base)
	// Under before over, and sub before sup; MathML expects both in that
	// order.
	if c.Bottom != nil {
		e.renderMathGroup(c.Bottom)
	}
	if c.Top != nil {
		e.renderMathGroup(c.Top)
	}
	e.End(tag)
}

// renderMathLr writes a delimited group as one row, with the delimiter at
// either end written as an operator that stretches to what it encloses.
func (e *Encoder) renderMathLr(c *value.MathLr) {
	// The delimiters are in the body along with what they wrap, so they have to
	// be separated out of it. c.Size is dropped: MathML cannot express a
	// delimiter size, and the browser sizes delimiters to their contents.
	items := flattenMath(c.Body)
	var open, closing value.Content
	if len(items) > 0 && isMathDelim(items[0]) {
		open, items = items[0], items[1:]
	}
	if len(items) > 0 && isMathDelim(items[len(items)-1]) {
		closing, items = items[len(items)-1], items[:len(items)-1]
	}

	stretch := tallMath(items...)
	outer := e.mathStretch
	e.mathStretch = stretch

	e.Start("mrow")
	if open != nil {
		e.renderMathDelim(open, stretch)
	}
	if len(items) > 0 {
		e.renderMathRow(items)
	}
	if closing != nil {
		e.renderMathDelim(closing, stretch)
	}
	e.End("mrow")

	e.mathStretch = outer
}

// tallMath reports whether any of items is taller than a line of ordinary
// symbols, and so whether delimiters around them should stretch.
//
// This has to be decided rather than always stretching, because a browser draws
// a stretchy delimiter enlarged and centered on the math axis even around a
// single letter, which around `x` is visibly too large. Only constructs that
// really are taller than a line, such as a fraction, a root, or a matrix, need
// a delimiter that grows.
// tallKinds are the constructs taller than a line, and so the ones whose
// delimiters stretch.
var tallKinds = value.SetOf(
	value.KindMathFrac,
	value.KindMathRoot,
	value.KindMathVec,
	value.KindMathCases,
	value.KindMathMat,
)

func tallMath(items ...value.Content) bool {
	for _, item := range items {
		if item == nil {
			continue
		}
		for range value.Preorder(item, tallKinds) {
			return true
		}
	}
	return false
}

// renderMathMatrix writes rows as a table between two delimiters. closing is
// empty for the one-sided brace of a cases construct.
func (e *Encoder) renderMathMatrix(open, closing, align string, rows [][]value.Content) {
	outer := e.mathStretch
	e.mathStretch = true

	e.Start("mrow")
	e.Start("mo", Attr{"stretchy", "true"})
	e.Text(open)
	e.End("mo")
	e.Start("mtable", Attr{"columnalign", align})
	for _, row := range rows {
		e.Start("mtr")
		for _, cell := range row {
			e.Start("mtd")
			e.renderMathGroup(cell)
			e.End("mtd")
		}
		e.End("mtr")
	}
	e.End("mtable")
	if closing != "" {
		e.Start("mo", Attr{"stretchy", "true"})
		e.Text(closing)
		e.End("mo")
	}
	e.End("mrow")

	e.mathStretch = outer
}

// renderMathDelim writes a delimiter of a delimited group. stretch says whether
// it grows to the height of what it encloses; see [tallMath] for why that is
// not always wanted.
func (e *Encoder) renderMathDelim(c value.Content, stretch bool) {
	t, ok := c.(*value.MathText)
	if !ok {
		e.renderMathGroup(c)
		return
	}
	e.Start("mo", Attr{"stretchy", strconv.FormatBool(stretch)})
	e.Text(t.Text)
	e.End("mo")
}

// mathDelims holds the characters that stretch when they open or close a
// delimited group. Typst tells them apart by Unicode math class; this is the
// part of that table the math elements can produce.
var mathDelims = map[string]bool{
	"(": true, ")": true,
	"[": true, "]": true,
	"{": true, "}": true,
	"⟨": true, "⟩": true,
	"⟦": true, "⟧": true,
	"⌊": true, "⌋": true,
	"⌈": true, "⌉": true,
	"|": true, "‖": true,
}

// isMathDelim reports whether c is a delimiter character, the kind that
// stretches at the edge of a delimited group.
func isMathDelim(c value.Content) bool {
	t, ok := c.(*value.MathText)
	return ok && mathDelims[t.Text]
}

// renderMathText writes a math leaf: a number, a variable, or an operator.
func (e *Encoder) renderMathText(c *value.MathText) {
	if c.Text == "" {
		return
	}
	if strings.TrimSpace(c.Text) == "" {
		e.Start("mtext")
		e.Text(c.Text)
		e.End("mtext")
		return
	}

	text, normal := styledMathText(c)
	tag := mathTextTag(c.Text)
	if tag == "mi" && normal {
		e.Start("mi", Attr{"mathvariant", "normal"})
	} else {
		e.Start(tag)
	}
	e.Text(text)
	e.End(tag)
}

// mathTextTag picks the MathML element for a math leaf: <mn> for numbers, <mi>
// for names, <mo> for operators. The browser spaces the three differently, so
// the distinction affects the output.
func mathTextTag(s string) string {
	digits, letters, hasDigit := true, true, false
	for _, r := range s {
		switch {
		case unicode.IsDigit(r):
			hasDigit = true
			letters = false
		case r == '.' || r == ',':
			// A separator is part of a number only with digits on both sides;
			// on its own it is punctuation.
			letters = false
		case unicode.IsLetter(r):
			digits = false
		case r == ' ':
			// A space is allowed inside a name, so an operator spelled as two
			// words ("lim inf") stays a single <mi>.
			digits = false
		default:
			digits, letters = false, false
		}
	}
	switch {
	case digits && hasDigit:
		return "mn"
	case letters:
		return "mi"
	default:
		return "mo"
	}
}

// styledMathText applies a math leaf's font style by mapping each letter to its
// counterpart in the Mathematical Alphanumeric Symbols block. That block is how
// MathML Core expresses these styles, mathvariant having been removed for
// everything but "normal". The second return reports that remaining case, the
// one style with no characters of its own.
func styledMathText(c *value.MathText) (text string, normal bool) {
	bold := c.Bold == value.Bool(true)
	variant := ""
	if s, ok := c.Variant.(value.Str); ok {
		variant = string(s)
	}
	// A single letter is italicized and anything longer left upright, which
	// is what MathML already does with a bare <mi>, so the unstyled case
	// needs no work.
	italic, explicit := c.Italic.(value.Bool)
	if !explicit {
		italic = value.Bool(autoItalic(c.Text))
	}
	if !bold && variant == "" && !explicit {
		return c.Text, false
	}

	var sb strings.Builder
	for _, r := range c.Text {
		sb.WriteRune(styledRune(r, bold, bool(italic), variant))
	}
	text = sb.String()
	return text, !bool(italic) && autoItalic(text)
}

// autoItalic reports whether MathML would italicize s on its own: a single
// letter, and only one that has an italic form to be mapped to.
func autoItalic(s string) bool {
	rs := []rune(s)
	return len(rs) == 1 && unicode.IsLetter(rs[0]) && rs[0] < 0x1D400
}

// styledRune maps one character to its styled form, or returns it unchanged
// when there is none — a plus sign is a plus sign however bold it is asked to
// be.
func styledRune(r rune, bold, italic bool, variant string) rune {
	var base [3]rune // upper-case, lower-case, digit; 0 where the style has none
	switch variant {
	case "", "serif":
		switch {
		case bold && italic:
			base = [3]rune{0x1D468, 0x1D482, 0x1D7CE}
		case bold:
			base = [3]rune{0x1D400, 0x1D41A, 0x1D7CE}
		case italic:
			base = [3]rune{0x1D434, 0x1D44E, 0}
		default:
			return r
		}
	case "sans":
		switch {
		case bold && italic:
			base = [3]rune{0x1D63C, 0x1D656, 0x1D7EC}
		case bold:
			base = [3]rune{0x1D5D4, 0x1D5EE, 0x1D7EC}
		case italic:
			base = [3]rune{0x1D608, 0x1D622, 0x1D7E2}
		default:
			base = [3]rune{0x1D5A0, 0x1D5BA, 0x1D7E2}
		}
	case "cal":
		if bold {
			base = [3]rune{0x1D4D0, 0x1D4EA, 0}
		} else {
			base = [3]rune{0x1D49C, 0x1D4B6, 0}
		}
	case "frak":
		if bold {
			base = [3]rune{0x1D56C, 0x1D586, 0}
		} else {
			base = [3]rune{0x1D504, 0x1D51E, 0}
		}
	case "bb":
		base = [3]rune{0x1D538, 0x1D552, 0x1D7D8}
	case "mono":
		base = [3]rune{0x1D670, 0x1D68A, 0x1D7F6}
	default:
		return r
	}

	var styled rune
	switch {
	case r >= 'A' && r <= 'Z' && base[0] != 0:
		styled = base[0] + (r - 'A')
	case r >= 'a' && r <= 'z' && base[1] != 0:
		styled = base[1] + (r - 'a')
	case r >= '0' && r <= '9' && base[2] != 0:
		styled = base[2] + (r - '0')
	default:
		return r
	}
	if hole, ok := mathAlphanumericHoles[styled]; ok {
		// The block has gaps where a letter is already encoded elsewhere.
		return hole
	}
	return styled
}

// mathAlphanumericHoles maps the unassigned code points of the Mathematical
// Alphanumeric Symbols block to the letters that stand in for them, which were
// encoded in the Letterlike Symbols block first.
var mathAlphanumericHoles = map[rune]rune{
	0x1D455: 0x210E, // italic h
	0x1D49D: 0x212C, // script B
	0x1D4A0: 0x2130, // script E
	0x1D4A1: 0x2131, // script F
	0x1D4A3: 0x210B, // script H
	0x1D4A4: 0x2110, // script I
	0x1D4A7: 0x2112, // script L
	0x1D4A8: 0x2133, // script M
	0x1D4AD: 0x211B, // script R
	0x1D4BA: 0x212F, // script e
	0x1D4BC: 0x210A, // script g
	0x1D4C4: 0x2134, // script o
	0x1D506: 0x212D, // fraktur C
	0x1D50B: 0x210C, // fraktur H
	0x1D50C: 0x2111, // fraktur I
	0x1D515: 0x211C, // fraktur R
	0x1D51D: 0x2128, // fraktur Z
	0x1D53A: 0x2102, // double-struck C
	0x1D53F: 0x210D, // double-struck H
	0x1D545: 0x2115, // double-struck N
	0x1D547: 0x2119, // double-struck P
	0x1D548: 0x211A, // double-struck Q
	0x1D549: 0x211D, // double-struck R
	0x1D551: 0x2124, // double-struck Z
}

// flattenMath splices the sequences out of a math body, so that the elements of
// an equation form one list to walk, split, and wrap.
func flattenMath(c value.Content) []value.Content {
	seq, ok := c.(*value.Sequence)
	if !ok {
		return []value.Content{c}
	}
	var items []value.Content
	for _, child := range seq.Children {
		items = append(items, flattenMath(child)...)
	}
	return items
}

// splitMath splits items at every element sep reports, dropping the separators.
func splitMath(items []value.Content, sep func(value.Content) bool) [][]value.Content {
	parts := [][]value.Content{nil}
	for _, item := range items {
		if sep(item) {
			parts = append(parts, nil)
			continue
		}
		parts[len(parts)-1] = append(parts[len(parts)-1], item)
	}
	return parts
}

// hasMathRows reports whether an equation body is laid out in rows: it holds a
// linebreak, or an alignment point that needs a column to align at.
func hasMathRows(items []value.Content) bool {
	for _, item := range items {
		switch item.(type) {
		case *value.Linebreak, *value.MathAlignPoint:
			return true
		}
	}
	return false
}

// primes spells a run of prime marks. Unicode has a character for each of the
// first four, and they set better than the same number of single primes.
func primes(count int) string {
	switch count {
	case 1:
		return "′"
	case 2:
		return "″"
	case 3:
		return "‴"
	case 4:
		return "⁗"
	default:
		return strings.Repeat("′", count)
	}
}

// mathTextContent collects the characters of a math body. An <mo> holds text
// and nothing else, so an operator built from richer content contributes only
// what it spells.
// textKinds are the elements that carry characters of their own.
var textKinds = value.SetOf(value.KindMathText, value.KindText, value.KindRaw)

func mathTextContent(c value.Content) string {
	var sb strings.Builder
	for v := range value.Preorder(c, textKinds) {
		switch v := v.Node().(type) {
		case *value.MathText:
			sb.WriteString(v.Text)
		case *value.Text:
			sb.WriteString(v.Text)
		case *value.Raw:
			sb.WriteString(v.Text)
		}
	}
	return sb.String()
}

// takesLimits reports whether c is an operator whose attachments belong above
// and below it.
func takesLimits(c value.Content) bool {
	op, ok := c.(*value.MathOp)
	return ok && op.Limits
}

// rowsOf turns the cells of a vector or cases construct into one-cell rows.
func rowsOf(cells []value.Content) [][]value.Content {
	rows := make([][]value.Content, len(cells))
	for i, cell := range cells {
		rows[i] = []value.Content{cell}
	}
	return rows
}
