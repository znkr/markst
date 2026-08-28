// Package smartquote resolves the quotation marks a writst document leaves
// unresolved. A [znkr.io/writst/value.SmartQuote] element records only whether
// the author typed ' or "; which glyph it stands for depends on the content
// around it, and that is decided here, at presentation time.
package smartquote

import (
	"unicode"
	"unicode/utf8"

	"znkr.io/writst/value"
)

// The quotation glyphs a [Quoter] substitutes. They are the English set;
// writst does not model a text language, so there is nothing to switch on.
const (
	SingleOpen  = "‘" // ‘
	SingleClose = "’" // ’
	DoubleOpen  = "“" // “
	DoubleClose = "”" // ”

	apostrophe  = "’" // ’
	singlePrime = "′" // ′
	doublePrime = "″" // ″
)

// Quoter resolves [znkr.io/writst/value.SmartQuote] elements into quotation
// glyphs with zero lookahead. Drive it while walking realized content in
// document order, handing it every element reached — including the ones it has
// nothing to say about — and emitting what [Quoter.Advance] gives back:
//
//	var q smartquote.Quoter
//	for c := range walk(doc) {
//		out(q.Advance(c))
//		switch c := c.(type) {
//		case *value.Text:
//			out(escape(c.Text))
//		case *value.Linebreak:
//			out("<br>")
//		}
//	}
//
// The quoter reads block structure off the content itself, so a presenter never
// has to tell it where a quotation ends. The zero Quoter is ready to use.
type Quoter struct {
	depth  uint8  // number of quotations currently open
	kinds  uint32 // one bit per nesting level, set when that quote is a double
	before rune   // the last character written, 0 at the start of a block
}

// Advance moves the quoter over c and returns the text to emit in c's place:
// the resolved glyph when c is a [value.SmartQuote], and "" for every other
// element, which the presenter renders itself.
//
// Call it for every element reached, in document order, before descending into
// a body. Block-level content ([value.Content.IsBlock]) starts a new quotation
// context, so an unclosed quotation cannot leak from one paragraph, heading, or
// list item into the next.
func (q *Quoter) Advance(c value.Content) string {
	if c.IsBlock() {
		*q = Quoter{}
		return ""
	}
	switch c := c.(type) {
	case *value.Text:
		q.wrote(c.Text)
	case *value.SmartQuote:
		s := q.quote(c.Double)
		q.wrote(s)
		return s
	case *value.Raw:
		// Inline raw — a block raw never reaches here, it resets above. The
		// presenter emits this text verbatim, so the quote in `dog`'s follows
		// a g and is an apostrophe, not an opening quotation.
		q.wrote(c.Text)
	case *value.MathText:
		// Math renders as text too: the x in $x$ is what the quote in $x$'s
		// follows.
		q.wrote(c.Text)
	case *value.Ref:
		// A reference renders as text the quoter cannot see: either the
		// supplement it is about to descend into, which overwrites this, or a
		// label the presenter resolves on its own. Something visible lands
		// here either way, and a letter is the safe stand-in — it makes a
		// following quote an apostrophe or a close, never a prime.
		q.wrote("x")
	case *value.Linebreak:
		// A line break sits inside a paragraph: it is whitespace to the quote
		// that follows, not a boundary, so an open quotation survives it.
		q.wrote("\n")
	case *value.Parbreak:
		// A parbreak ends the paragraph it follows. Realization drops these
		// while grouping, so a presenter walking realized content will not meet
		// one, but a quoter driven over unrealized content still gets it right.
		*q = Quoter{}
	}
	return ""
}

// wrote records that s reached the output. Only the last character matters, so
// passing a long run of text is as cheap as passing a single character.
func (q *Quoter) wrote(s string) {
	if r, size := utf8.DecodeLastRuneInString(s); size > 0 {
		q.before = r
	}
}

func (q *Quoter) quote(double bool) string {
	opened, isOpen := q.top()
	before := q.before
	if before == 0 {
		before = ' '
	}

	// After a number, and without a quotation of this kind most recently
	// opened, this is a measurement: 5'11" is five feet eleven inches.
	if unicode.IsNumber(before) && (!isOpen || opened != double) {
		if double {
			return doublePrime
		}
		return singlePrime
	}

	// A single quote after a letter, with no single quotation open, is an
	// apostrophe: dog's, not dog‘s.
	if !double && (!isOpen || opened) && unicode.IsLetter(before) {
		return apostrophe
	}

	// The innermost quotation is of this kind and the preceding character
	// doesn't announce a nested quotation, so this closes it.
	if isOpen && opened == double && !unicode.IsSpace(before) && !isOpeningBracket(before) {
		q.pop()
		if double {
			return DoubleClose
		}
		return SingleClose
	}

	// Otherwise it opens a new quotation.
	q.push(double)
	if double {
		return DoubleOpen
	}
	return SingleOpen
}

// top reports whether the innermost open quotation is a double quote. ok is
// false when no quotation is open.
func (q *Quoter) top() (double, ok bool) {
	if q.depth == 0 {
		return false, false
	}
	return q.kinds&(1<<(q.depth-1)) != 0, true
}

// push opens a quotation. Nesting deeper than 32 levels stops being tracked;
// such a document has bigger problems than its quotation marks.
func (q *Quoter) push(double bool) {
	if q.depth >= 32 {
		return
	}
	if double {
		q.kinds |= 1 << q.depth
	}
	q.depth++
}

// pop closes the innermost quotation.
func (q *Quoter) pop() {
	q.depth--
	q.kinds &= 1<<q.depth - 1
}

func isOpeningBracket(r rune) bool {
	return r == '(' || r == '{' || r == '['
}
