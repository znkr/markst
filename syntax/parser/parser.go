// Copyright 2026 Florian Zenker (flo@znkr.io)
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package parser turns Markst source into a syntax tree.
//
// [Parse] returns a [syntax.RootNode] covering the whole source, whitespace and
// comments included, so the tree can be formatted back out as well as read.
//
// The parser drives the scanner, switching its mode between markup, math, and
// code as it enters and leaves each construct, and tracks newlines, which are
// significant in markup and sometimes end an expression in code.
//
// Invalid input never stops a parse. The parser puts [syntax.Error] nodes in
// the tree where it could not make sense of the source, and carries on.
package parser

import (
	"fmt"
	"slices"
	"unicode"

	"znkr.io/markst/syntax"
	"znkr.io/markst/syntax/scanner"
)

var stopParse = syntax.SetOf(syntax.KindEnd)

// Parse parses src as a Markst document. The root node is always of kind
// [syntax.KindMarkup] and carries the source, so any span in the tree resolves
// to text and to a line and column.
//
// Parse never fails. Invalid input appears in the tree as [syntax.Error] nodes.
func Parse(src []byte) syntax.RootNode {
	p := newParser(src)
	p.parseMarkup(stopParse, mfAtStart|mfWrapTrivia)
	if p.cur.kind != syntax.KindEnd {
		panic("expected end of input")
	}
	if len(p.nodes) != 1 {
		panic("expected single root node")
	}
	return syntax.RootNode{
		Src:    src,
		Source: p.s.Source(),
		Inner:  p.nodes[0].(*syntax.Inner),
	}
}

type nlMode int

const (
	nlContinue           nlMode = -iota // Continue scanning over newlines
	nlStop                              // Stop scanning when a newline is encountered
	nlStopAtParBreak                    // Stop scanning when a paragraph break is encountered
	nlContextualContinue                // Continue only if there is a continuation with `else` or `.` (Code only).
)

func nlColumn(col int) nlMode {
	if col <= 0 {
		return nlStop
	}
	return nlMode(col)
}

func (m nlMode) stopAt(t token) bool {
	if !t.newline {
		return false
	}
	switch m {
	case nlContinue:
		return false
	case nlStop:
		return true
	case nlStopAtParBreak:
		return t.parbreak
	case nlContextualContinue:
		return t.kind != syntax.KindElse && t.kind != syntax.KindDot
	default:
		if m < 0 {
			panic(fmt.Sprintf("unknown nlMode: %s", fmt.Sprint(m)))
		}
		return t.indent <= int(m)
	}
}

type parser struct {
	s *scanner.Scanner

	// src is the source being parsed, which is where a node's text comes from;
	// see [parser.text].
	src []byte

	// a is the scanner's arena: the nodes the parser builds go in the same
	// blocks as the tokens they are built over.
	a *syntax.Arena

	// state
	cur            token
	nodes          []syntax.Node
	newlineMode    nlMode
	memos          map[int]memo
	bracketNesting int

	// depth is how many levels of nesting the parser has descended into; see
	// [maxDepth].
	depth int

	// errAnchor is the node index of the error most recently produced by
	// [parser.expected] or [parser.errorf], or noAnchor when that call reused
	// an error node already present. Read it through [parser.errorMarker]; it
	// is meaningful only immediately after the call.
	errAnchor int
}

// noAnchor marks the absence of a fresh error node; see [parser.errAnchor].
const noAnchor = -1

type token struct {
	kind     syntax.Kind
	node     syntax.Node
	start    int
	prevEnd  int
	trivia   int
	newline  bool
	parbreak bool
	indent   int
}

func newParser(src []byte) *parser {
	s := scanner.New(src)
	p := &parser{
		s:   s,
		src: src,
		a:   s.Arena(),
		// The working stack holds every node parsed but not yet wrapped, which
		// at the top level is the whole document. Sizing it from the source
		// avoids repeatedly doubling it from nothing: the article the
		// benchmarks parse peaks at one entry per 23 source bytes, so one per
		// 16 leaves room without much waste.
		nodes:     make([]syntax.Node, 0, min(len(src)/16+16, 4096)),
		memos:     make(map[int]memo),
		errAnchor: noAnchor,
	}
	p.next()
	return p
}

func (p *parser) at(kind syntax.Kind) bool {
	return p.cur.kind == kind
}

func (p *parser) atSet(kinds syntax.Set) bool {
	return kinds.Contains(p.cur.kind)
}

// atEnd reports whether the parser is at the end of the input, as opposed to
// the end a newline mode fakes to stop the current construct.
func (p *parser) atEnd() bool {
	return p.cur.node.Kind() == syntax.KindEnd
}

func (p *parser) directlyAt(kind syntax.Kind) bool {
	return p.cur.kind == kind && p.cur.trivia == 0
}

func (p *parser) next() {
	p.cur = token{
		prevEnd: p.s.Offset(),
		start:   p.s.Offset(),
	}
	for {
		p.cur.kind, p.cur.node = p.s.Next()
		if !syntax.Trivia.Contains(p.cur.kind) {
			break
		}
		p.cur.start = p.s.Offset()
		p.cur.newline = p.cur.newline || p.s.Newline()
		p.cur.parbreak = p.cur.parbreak || p.cur.kind == syntax.KindParbreak
		if p.s.Newline() {
			p.cur.indent = p.s.Column()
		}
		p.nodes = append(p.nodes, p.cur.node)
		p.cur.trivia++
	}

	if p.newlineMode.stopAt(p.cur) {
		p.cur.kind = syntax.KindEnd // treat as end of input to make the caller stop
	}
}

// text returns the source n covers.
func (p *parser) text(n syntax.Node) []byte { return syntax.Text(p.src, n) }

func (p *parser) consume() {
	p.nodes = append(p.nodes, p.cur.node)
	p.next()
}

func (p *parser) consumeIf(kind syntax.Kind) bool {
	if p.at(kind) {
		p.consume()
		return true
	}
	return false
}

// expected reports that the given construct was expected at the current
// position. If the current token is already a scanner error, it is consumed
// (after trimming preceding zero-width errors) so the token is recorded in
// the correct lexing mode. Otherwise, it deduplicates: if there is already
// an error node immediately before the trivia, it returns that existing
// error instead of creating a new one. In the common case it inserts a
// zero-width error without consuming the current token, so the caller can
// continue recovery from the same position.
func (p *parser) expected(expected string) *syntax.Error {
	if p.at(syntax.KindError) {
		// Consume erroneous tokens so they are recorded in the correct
		// lexing mode (important for incremental reparsing).
		p.trimErrors()
		e := p.cur.node.(*syntax.Error)
		p.consume()
		p.errAnchor = len(p.nodes) - p.cur.trivia - 1
		return e
	}
	at := len(p.nodes) - p.cur.trivia
	if at > 0 && p.nodes[at-1].Kind() == syntax.KindError {
		// Already have an error at this position. It belongs to whatever was
		// parsed before, so a caller repairing the current construct must not
		// pull it in.
		p.errAnchor = noAnchor
		return p.nodes[at-1].(*syntax.Error)
	}
	var span syntax.Span
	if at > 0 {
		span = p.nodes[at-1].Span()
	}
	n := p.a.Error(
		syntax.Span{Start: span.End, End: span.End},
		"expected "+expected,
	)
	p.nodes = slices.Insert(p.nodes, at, syntax.Node(n))
	p.errAnchor = at
	return n
}

// errorMarker re-anchors a wrap marker after an error-recovery call.
//
// [parser.expected] inserts its zero-width error before the trivia
// [parser.next] already appended, which shifts every index at or after it, so a
// marker taken on entry to the current construct no longer points at that
// construct's first node. When a fresh error node was produced, it is the only
// thing the construct parsed, so the marker moves onto it and the error ends up
// inside the wrapped node. When [parser.expected] reused an error already
// present, that error belongs to an earlier construct and m is kept.
func (p *parser) errorMarker(m int) int {
	if p.errAnchor == noAnchor {
		return m
	}
	return p.errAnchor
}

// errorf converts the current token into an error node with the given message,
// appends it to the node list, and advances the scanner. This consumes the
// problematic token so parsing can continue past it. Use this when the current
// token itself is the error and should be absorbed into the tree.
func (p *parser) errorf(format string, args ...any) *syntax.Error {
	err := p.asErrorNode(p.cur.node, format, args...)
	p.nodes = append(p.nodes, syntax.Node(err))
	p.next()
	p.errAnchor = len(p.nodes) - p.cur.trivia - 1
	return err
}

func (p *parser) consumeAs(kind syntax.Kind) {
	if p.at(syntax.KindError) {
		panic("cannot convert error node")
	}
	p.cur.node = p.a.Convert(p.cur.node, kind)
	p.consume()
}

func (p *parser) assert(expected syntax.Kind) {
	if p.cur.kind != expected {
		panic(fmt.Sprintf("expected %s, got %s", expected.Name(), p.cur.kind.Name()))
	}
	p.consume()
}

// expect consumes the current token if it matches kind and returns true.
// Otherwise, it delegates to [parser.expected] which inserts a zero-width
// error without consuming the current token (or consumes it if it's already
// a scanner error). This lets the enclosing construct continue recovery
// from the same position.
func (p *parser) expect(kind syntax.Kind) bool {
	if p.cur.kind == kind {
		p.consume()
		p.errAnchor = noAnchor
		return true
	} else if kind == syntax.KindIdent && syntax.Keywords.Contains(p.cur.kind) {
		p.trimErrors()
		n := p.cur
		e := p.errorf("expected %s", kind.Name())
		e.Hint(fmt.Sprintf("%s is not allowed as an identifier; try `%s_` instead", n.kind.Name(), p.text(n.node)))
		return false
	} else {
		p.expected(kind.Name())
		return false
	}
}

// expectedAt replaces the node at index i with an error node, reporting that
// the given construct was expected but the existing node was found instead. This
// is for an error found after the fact: something parsed speculatively that
// turned out to be invalid, such as a complex expression where only an
// identifier is allowed. The error takes the replaced node's span.
func (p *parser) expectedAt(i int, expected string) *syntax.Error {
	cur := p.nodes[i]
	n := p.asErrorNode(cur, "expected %s, found %s", expected, cur.Kind().Name())
	p.nodes[i] = syntax.Node(n)
	return n
}

// unexpected reports and consumes the current token as an unexpected error. It
// first trims any trailing zero-width errors (via trimErrors) to avoid
// cascading "expected X" messages at the same position. The token is consumed
// so the parser can make forward progress past the unexpected input.
func (p *parser) unexpected() *syntax.Error {
	p.trimErrors()
	return p.errorf("unexpected %s", p.cur.kind.Name())
}

// trimErrors removes trailing zero-width error nodes that might be left from previous error
// handling, as they would otherwise result in cascading errors at the same position.
func (p *parser) trimErrors() {
	end := len(p.nodes) - p.cur.trivia
	start := end
	for ; start > 0; start-- {
		n := p.nodes[start-1]
		if n.Kind() != syntax.KindError || n.Span().Start != n.Span().End {
			break
		}
	}
	p.nodes = slices.Delete(p.nodes, start, end)
}

// expectClosing attempts to consume a closing delimiter (e.g., ')', ']', '}').
// If the current token matches, it is consumed and nil is returned. Otherwise,
// the opening delimiter at index open is retroactively converted to an
// "unclosed delimiter" error — this places the error at the opening position
// rather than at the current (possibly far away) position, which produces
// better diagnostics. If the opening node is already an error, no additional
// error is created.
func (p *parser) expectClosing(open int, expected syntax.Kind) *syntax.Error {
	if p.cur.kind == expected {
		p.consume()
		return nil
	}
	if p.nodes[open].Kind() == syntax.KindError {
		// Already have an error at this position.
		return nil
	}
	n := p.asErrorNode(p.nodes[open], "unclosed delimiter")
	p.nodes[open] = n
	return n
}

func (p *parser) flushTrivia() {
	p.cur.trivia = 0
	p.cur.prevEnd = p.cur.start
}

func (p *parser) wrap(start int, kind syntax.Kind) {
	to := len(p.nodes) - p.cur.trivia
	from := min(start, to)
	var wrapped syntax.Node
	if from == to {
		// Nothing to wrap. The node goes between the preceding node and the
		// trivia after it, so it spans nothing. It needs the offset passed in,
		// since a node with no children has nothing to derive a span from.
		var pos uint32
		if from > 0 {
			pos = p.nodes[from-1].Span().End
		}
		wrapped = p.a.EmptyInner(kind, pos)
	} else {
		wrapped = p.a.Inner(kind, p.a.CloneNodes(p.nodes[from:to]))
		p.nodes = slices.Delete(p.nodes, from, to)
	}
	p.nodes = slices.Insert(p.nodes, from, wrapped)
}

func (p *parser) withMode(mode syntax.Mode, nlmode nlMode, fn func()) {
	prev := p.s.Mode()
	p.s.SetMode(mode)
	p.withNewlineMode(nlmode, fn)
	if mode != prev {
		p.s.SetMode(prev)
		// Rescan the last token: the mode just changed, and the same characters
		// may tokenize differently in the new one.
		p.s.Seek(p.cur.prevEnd)
		p.nodes = p.nodes[:len(p.nodes)-p.cur.trivia]
		p.next()
	}
}

func (p *parser) withNewlineMode(mode nlMode, fn func()) {
	prevMode := p.newlineMode
	p.newlineMode = mode
	fn()
	p.newlineMode = prevMode
	if p.cur.newline && prevMode != mode {
		p.cur.kind = p.cur.node.Kind() // restore correct kind
		if p.newlineMode.stopAt(p.cur) {
			p.cur.kind = syntax.KindEnd // new mode treads this as end of input
		}
	}
}

type memo struct {
	nodes []syntax.Node
	state partialState
}

type partialState struct {
	offset      int
	scannerMode syntax.Mode
	token       token
}

type checkpoint struct {
	nodes int
	state partialState
}

func (p *parser) checkpoint() checkpoint {
	return checkpoint{
		nodes: len(p.nodes),
		state: partialState{
			offset:      p.s.Offset(),
			scannerMode: p.s.Mode(),
			token:       p.cur,
		},
	}
}

func (p *parser) restore(cp checkpoint) {
	p.nodes = p.nodes[:cp.nodes]
	p.restorePartial(cp.state)
}

func (p *parser) restorePartial(state partialState) {
	p.s.Seek(state.offset)
	p.s.SetMode(state.scannerMode)
	p.cur = state.token
}

func (p *parser) memoizeNodes(key, start int) {
	cp := p.checkpoint()
	p.memos[key] = memo{
		nodes: slices.Clone(p.nodes[start:]),
		state: cp.state,
	}
}

func (p *parser) restoreMemo(key int) bool {
	memo, ok := p.memos[key]
	if !ok {
		return false
	}
	p.nodes = append(p.nodes, memo.nodes...)
	p.restorePartial(memo.state)
	return true
}

type markupFlags int

const (
	mfAtStart markupFlags = 1 << iota
	mfWrapTrivia
)

func (p *parser) parseMarkup(stops syntax.Set, flags markupFlags) {
	start := len(p.nodes)
	if flags&mfWrapTrivia != 0 || p.cur.parbreak {
		// A parbreak is markup content, not trivia. One buffered before the
		// body starts, as in a list or enum item whose content begins on a
		// later line ("- \n\n  x"), belongs inside the body rather than
		// beside it, where the analyzer would find it instead of the body's
		// Markup node.
		start -= p.cur.trivia
	}
	p.parseMarkupExprs(stops, flags&mfAtStart != 0)
	if flags&mfWrapTrivia != 0 {
		p.flushTrivia()
	}
	p.wrap(start, syntax.KindMarkup)
}

// parseMarkupExprs parses markup expressions until a token in stops. Nesting is
// counted here rather than per expression, because every markup construct
// containing markup passes through it.
func (p *parser) parseMarkupExprs(stops syntax.Set, atStart bool) {
	if !p.enterDepth(stops) {
		return
	}
	defer p.leaveDepth()

	atStart = p.cur.newline || atStart
	// ifAtStart calls parseFn when at the start of a line, otherwise
	// consumes the current token as plain text.
	ifAtStart := func(parseFn func()) {
		if atStart {
			parseFn()
		} else {
			p.consumeAs(syntax.KindText)
		}
	}
	for !p.atSet(stops) {
		switch p.cur.kind {
		case syntax.KindLeftBracket:
			p.bracketNesting++
			p.consumeAs(syntax.KindText)
		case syntax.KindRightBracket:
			if p.bracketNesting > 0 {
				p.bracketNesting--
				p.consumeAs(syntax.KindText)
			} else {
				err := p.unexpected()
				err.Hint("try using a backslash escape: \\]")
			}
		case syntax.KindStar:
			p.parseStrong()
		case syntax.KindUnderscore:
			p.parseEmph()
		case syntax.KindHeadingMarker:
			ifAtStart(p.parseHeading)
		case syntax.KindListMarker:
			ifAtStart(p.parseListItem)
		case syntax.KindEnumMarker:
			ifAtStart(p.parseEnumItem)
		case syntax.KindTermMarker:
			ifAtStart(p.parseTermItem)
		case syntax.KindHash:
			p.parseEmbeddedCodeExpr()
		case syntax.KindRefMarker:
			p.parseReference()
		case syntax.KindDollar:
			p.parseEquation()
		case syntax.KindText, syntax.KindLinebreak, syntax.KindEscape, syntax.KindShorthand, syntax.KindSmartQuote, syntax.KindLink, syntax.KindLabel, syntax.KindRaw:
			p.consume()
		case syntax.KindColon:
			p.consumeAs(syntax.KindText)
		default:
			p.unexpected()
		}

		atStart = p.cur.newline
	}
}

func (p *parser) parseCode(stops syntax.Set) {
	start := len(p.nodes) - p.cur.trivia
	// Nesting is counted in parseCodeExprPrec. The check here only stops a
	// parse that is already too deep from reporting one error per
	// expression.
	if p.checkDepth(stops) {
		for !p.atSet(stops) {
			p.withNewlineMode(nlContextualContinue, func() {
				if !p.atSet(syntax.CodeExpr) {
					p.unexpected()
					return
				}
				p.parseCodeExpr()
				if !p.atSet(stops) && !p.consumeIf(syntax.KindSemicolon) {
					err := p.expected("semicolon or line break")
					if p.at(syntax.KindLabel) {
						err.Hint("labels can only be applied in markup mode")
						err.Hint("try wrapping your code in a markup block (`[ ]`)")
					}
				}
			})
		}
	}
	p.flushTrivia()
	p.wrap(start, syntax.KindCode)
}

var strongStops = syntax.SetOf(syntax.KindStar, syntax.KindRightBracket, syntax.KindEnd)

func (p *parser) parseStrong() {
	p.withNewlineMode(nlStopAtParBreak, func() {
		start := len(p.nodes)
		p.assert(syntax.KindStar)
		p.parseMarkup(strongStops, mfWrapTrivia)
		p.expectClosing(start, syntax.KindStar)
		p.wrap(start, syntax.KindStrong)
	})
}

var emphStops = syntax.SetOf(syntax.KindUnderscore, syntax.KindRightBracket, syntax.KindEnd)

func (p *parser) parseEmph() {
	p.withNewlineMode(nlStopAtParBreak, func() {
		start := len(p.nodes)
		p.assert(syntax.KindUnderscore)
		p.parseMarkup(emphStops, mfWrapTrivia)
		p.expectClosing(start, syntax.KindUnderscore)
		p.wrap(start, syntax.KindEmph)
	})
}

var headingStops = syntax.SetOf(syntax.KindLabel, syntax.KindRightBracket, syntax.KindEnd)

func (p *parser) parseHeading() {
	p.withNewlineMode(nlStop, func() {
		start := len(p.nodes)
		p.assert(syntax.KindHeadingMarker)
		p.parseMarkup(headingStops, 0)
		p.wrap(start, syntax.KindHeading)
	})
}

var listStops = syntax.SetOf(syntax.KindRightBracket, syntax.KindEnd)

func (p *parser) parseListItem() {
	p.withNewlineMode(nlColumn(p.s.Column()), func() {
		start := len(p.nodes)
		p.assert(syntax.KindListMarker)
		p.parseMarkup(listStops, mfAtStart)
		p.wrap(start, syntax.KindListItem)
	})
}

func (p *parser) parseEnumItem() {
	p.withNewlineMode(nlColumn(p.s.Column()), func() {
		start := len(p.nodes)
		p.assert(syntax.KindEnumMarker)
		p.parseMarkup(listStops, mfAtStart)
		p.wrap(start, syntax.KindEnumItem)
	})
}

var termStops = syntax.SetOf(syntax.KindColon, syntax.KindRightBracket, syntax.KindEnd)

func (p *parser) parseTermItem() {
	p.withNewlineMode(nlColumn(p.s.Column()), func() {
		start := len(p.nodes)
		p.withNewlineMode(nlStop, func() {
			p.assert(syntax.KindTermMarker)
			p.parseMarkup(termStops, 0)
		})
		p.expect(syntax.KindColon)
		p.parseMarkup(listStops, mfAtStart)
		p.wrap(start, syntax.KindTermItem)
	})

}

func (p *parser) parseEmbeddedCodeExpr() {
	p.withMode(syntax.ModeCode, nlStop, func() {
		p.assert(syntax.KindHash)
		if p.cur.trivia > 0 || p.at(syntax.KindEnd) {
			p.expected("expression")
			return
		}

		stmt := p.atSet(syntax.Stmts)
		p.parseCodeExprPrec(true, 0)

		semi := (stmt || p.directlyAt(syntax.KindSemicolon)) && p.consumeIf(syntax.KindSemicolon)
		if stmt && !semi && !p.at(syntax.KindEnd) && !p.at(syntax.KindRightBracket) {
			p.expected("semicolon or line break")
		}
	})
}

// parseReference parses a reference node @target or @target[...].
func (p *parser) parseReference() {
	start := len(p.nodes)
	p.assert(syntax.KindRefMarker)
	if p.directlyAt(syntax.KindLeftBracket) {
		p.parseContentBlock()
	}
	p.wrap(start, syntax.KindRef)
}

var mathStops = syntax.SetOf(syntax.KindDollar, syntax.KindEnd)

// parseEquation parses a mathematical equation: `$x$`, `$ x^2 $`.
func (p *parser) parseEquation() {
	start := len(p.nodes)
	p.withMode(syntax.ModeMath, nlContinue, func() {
		p.assert(syntax.KindDollar)
		p.parseMath(mathStops)
		p.expectClosing(start, syntax.KindDollar)
	})
	p.wrap(start, syntax.KindEquation)
}

// parseMath parses the contents of an equation, wrapping them in a Math node.
func (p *parser) parseMath(stops syntax.Set) {
	start := len(p.nodes)
	p.parseMathExprs(stops)
	p.wrap(start, syntax.KindMath)
}

// parseMathExprs parses a sequence of math expressions, returning the count
// parsed (including errors).
func (p *parser) parseMathExprs(stops syntax.Set) int {
	// Nesting is counted in parseMathExprPrec. The check here only stops a
	// parse that is already too deep from reporting one error per
	// expression. It reports a count of 1 so the caller treats the error
	// node left behind as the single expression it stands for.
	if !p.checkDepth(stops) {
		return 1
	}
	count := 0
	for !p.atSet(stops) {
		if p.atSet(syntax.MathExpr) {
			p.parseMathExpr()
		} else {
			p.unexpected()
		}
		count++
	}
	return count
}

func (p *parser) parseMathExpr() {
	p.parseMathExprPrec(0, syntax.Set{})
}

const (
	mathFuncPrec = 2
	mathRootPrec = 2
)

type mathAssoc int

const (
	mathAssocNone  mathAssoc = iota // postfix operator: no right operand
	mathAssocLeft                   // left-associative infix operator
	mathAssocRight                  // right-associative infix operator
)

// mathOp reports the wrapper kind, associativity and precedence of the math
// operator kind, or ok=false if the kind is not a math operator.
func mathOp(kind syntax.Kind, hadTrivia bool) (wrapper syntax.Kind, assoc mathAssoc, prec int, ok bool) {
	switch kind {
	case syntax.KindSlash:
		return syntax.KindMathFrac, mathAssocLeft, 1, true
	case syntax.KindUnderscore, syntax.KindHat:
		return syntax.KindMathAttach, mathAssocRight, 2, true
	case syntax.KindMathPrimes:
		if !hadTrivia {
			return syntax.KindMathAttach, mathAssocNone, 2, true
		}
	case syntax.KindBang:
		if !hadTrivia {
			return syntax.KindMath, mathAssocNone, 3, true
		}
	}
	return syntax.KindInvalid, 0, 0, false
}

// attachChainSet returns the set of attachment operators that may chain after
// op (`^` chains with `_`, `_` with `^`, primes with either).
func attachChainSet(op syntax.Kind) syntax.Set {
	switch op {
	case syntax.KindHat:
		return syntax.SetOf(syntax.KindUnderscore)
	case syntax.KindUnderscore:
		return syntax.SetOf(syntax.KindHat)
	default: // primes
		return syntax.SetOf(syntax.KindHat, syntax.KindUnderscore)
	}
}

// parseMathExprPrec parses a math expression with at least the given
// precedence, chaining infix and postfix operators (attachment, fraction,
// root). stopSet holds kinds at which to stop (used to constrain attachment
// chains).
func (p *parser) parseMathExprPrec(minPrec int, stopSet syntax.Set) {
	if !p.enterDepth(noStops) {
		return
	}

	m := len(p.nodes)
	continuable := false
	switch p.cur.kind {
	case syntax.KindHash:
		p.parseEmbeddedCodeExpr()

	// The scanner produces full FieldAccess nodes where needed.
	case syntax.KindMathIdent, syntax.KindFieldAccess:
		continuable = true
		p.consume()
		// A function call for an identifier or field access.
		if mathFuncPrec >= minPrec && p.directlyAt(syntax.KindLeftParen) {
			p.parseMathArgs()
			p.wrap(m, syntax.KindMathCall)
			continuable = false
		}

	case syntax.KindLeftBrace, syntax.KindLeftParen:
		p.parseMathDelimited()

	case syntax.KindRightBrace:
		if string(p.text(p.cur.node)) == "|]" {
			p.consumeAs(syntax.KindMathShorthand)
		} else {
			p.consumeAs(syntax.KindMathText)
		}

	case syntax.KindDot, syntax.KindBang, syntax.KindComma, syntax.KindSemicolon, syntax.KindRightParen:
		p.consumeAs(syntax.KindMathText)

	case syntax.KindMathText:
		continuable = isMathAlphabetic(p.text(p.cur.node))
		p.consume()

	case syntax.KindLinebreak, syntax.KindMathAlignPoint, syntax.KindMathShorthand:
		p.consume()

	case syntax.KindMathPrimes, syntax.KindEscape, syntax.KindStr:
		continuable = true
		p.consume()

	case syntax.KindRoot:
		p.consume()
		m2 := len(p.nodes)
		p.parseMathExprPrec(mathRootPrec, syntax.Set{})
		p.mathUnparen(m2)
		p.wrap(m, syntax.KindMathRoot)

	default:
		p.expected("expression")
		m = p.errorMarker(m)
	}

	// An implicit function call: a continuable token directly followed by
	// delimiters groups with function precedence, so `a(b)/c` parses as
	// `(a(b))/c` when `a` is continuable.
	if continuable && mathFuncPrec >= minPrec && p.cur.trivia == 0 &&
		(p.at(syntax.KindLeftBrace) || p.at(syntax.KindLeftParen)) {
		p.parseMathDelimited()
		p.wrap(m, syntax.KindMath)
	}

	// Parse infix and postfix operators.
	for !p.atSet(stopSet) {
		opKind := p.cur.kind
		hadTrivia := p.cur.trivia > 0
		wrapper, assoc, prec, ok := mathOp(opKind, hadTrivia)
		if !ok || prec < minPrec {
			break
		}

		var chainSet syntax.Set
		if wrapper == syntax.KindMathAttach {
			chainSet = attachChainSet(opKind)
		}

		// Eat the operator.
		if opKind == syntax.KindBang {
			p.consumeAs(syntax.KindMathText)
		} else {
			p.consume()
		}

		// Slash removes parens from its left operand.
		if wrapper == syntax.KindMathFrac {
			p.mathUnparen(m)
		}

		// Parse the right operand.
		if assoc != mathAssocNone {
			rprec := prec
			if assoc == mathAssocLeft {
				rprec = prec + 1
			}
			mRhs := len(p.nodes)
			p.parseMathExprPrec(rprec, chainSet)
			p.mathUnparen(mRhs)
		}

		// Do not interrupt a chain when first parsing a prime: `a^b'_c^d`
		// groups as `(a^(b')_c)^d`, not `a^(b'_c^d)`.
		if !(opKind == syntax.KindMathPrimes && p.atSet(stopSet)) {
			for p.atSet(chainSet) {
				chainSet = chainSet.Remove(p.cur.kind)
				p.consume()
				mChainRhs := len(p.nodes)
				p.parseMathExprPrec(prec, chainSet)
				p.mathUnparen(mChainRhs)
			}
		}

		p.wrap(m, wrapper)
	}
	p.leaveDepth()
}

// isMathAlphabetic reports whether text counts as alphabetic in math, which
// causes it to group with parens as an implicit function call.
func isMathAlphabetic(text []byte) bool {
	if len(text) == 0 {
		return false
	}
	for _, r := range string(text) {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return true
}

var mathDelimitedStops = syntax.SetOf(syntax.KindDollar, syntax.KindEnd, syntax.KindRightBrace, syntax.KindRightParen)

// parseMathDelimited parses matched delimiters in math: `[x + y]`. The scanner
// produces `{Left,Right}{Brace,Paren}` for delimiters, which are converted back
// to MathText or MathShorthand before being eaten.
func (p *parser) parseMathDelimited() {
	m := len(p.nodes)
	if string(p.text(p.cur.node)) == "[|" {
		p.consumeAs(syntax.KindMathShorthand)
	} else {
		p.consumeAs(syntax.KindMathText)
	}
	mBody := len(p.nodes)
	p.parseMathExprs(mathDelimitedStops)
	if p.at(syntax.KindRightBrace) || p.at(syntax.KindRightParen) {
		p.wrap(mBody, syntax.KindMath)
		if string(p.text(p.cur.node)) == "|]" {
			p.consumeAs(syntax.KindMathShorthand)
		} else {
			p.consumeAs(syntax.KindMathText)
		}
		p.wrap(m, syntax.KindMathDelimited)
	} else {
		// No closing delimiter: just produce a math sequence.
		p.wrap(m, syntax.KindMath)
	}
}

// mathUnparen removes one set of parentheses (if any) from a previously parsed
// expression at marker m by converting the delimited node to a Math node with
// its `(`/`)` re-kinded as parens.
func (p *parser) mathUnparen(m int) {
	if m >= len(p.nodes) {
		return
	}
	inner, ok := p.nodes[m].(*syntax.Inner)
	if !ok || inner.Kind() != syntax.KindMathDelimited {
		return
	}
	children := inner.Children()
	if len(children) < 2 {
		return
	}
	first, last := children[0], children[len(children)-1]
	if string(p.text(first)) == "(" && string(p.text(last)) == ")" {
		newChildren := p.a.CloneNodes(children)
		newChildren[0] = p.a.Convert(first, syntax.KindLeftParen)
		newChildren[len(newChildren)-1] = p.a.Convert(last, syntax.KindRightParen)
		p.nodes[m] = p.a.Inner(syntax.KindMath, newChildren)
	}
}

var mathArgsStops = syntax.SetOf(syntax.KindEnd, syntax.KindDollar, syntax.KindRightParen)
var mathArgStops = syntax.SetOf(syntax.KindEnd, syntax.KindDollar, syntax.KindComma, syntax.KindSemicolon, syntax.KindRightParen)

// parseMathArgs parses an argument list in math: `(a, b; c, d; size: #50%)`.
func (p *parser) parseMathArgs() {
	m := len(p.nodes)
	p.assert(syntax.KindLeftParen)

	seen := make(map[string]bool)
	for !p.atSet(mathArgsStops) {
		p.parseMathArg(seen)
		switch p.cur.kind {
		case syntax.KindEnd, syntax.KindDollar, syntax.KindRightParen:
			// Terminator: handled by the loop condition.
		case syntax.KindSemicolon, syntax.KindComma:
			p.consume()
		default:
			p.expected("comma or semicolon")
		}
	}

	p.expectClosing(m, syntax.KindRightParen)
	p.wrap(m, syntax.KindMathArgs)
}

// parseMathArg parses a single argument in a math argument list, handling
// spread (`..x`) and named (`name: value`) arguments.
func (p *parser) parseMathArg(seen map[string]bool) {
	m := len(p.nodes)
	start := p.cur.start
	argKind := syntax.KindInvalid

	if node := p.s.MaybeMathSpreadArg(start); node != nil {
		argKind = syntax.KindSpread
		p.cur.node = node
		p.cur.kind = syntax.KindDots
		p.consume()
	} else if node := p.s.MaybeMathNamedArg(start); node != nil {
		argKind = syntax.KindNamed
		p.cur.node = node
		p.cur.kind = node.Kind()
		text := string(p.text(node))
		p.consume()
		p.consumeAs(syntax.KindColon)
		if seen[text] {
			prev := p.nodes[m]
			p.nodes[m] = p.a.Error(prev.Span(), fmt.Sprintf("duplicate argument: %s", text))
		}
		seen[text] = true
	}

	mArg := len(p.nodes)
	count := p.parseMathExprs(mathArgStops)
	if count == 0 && argKind == syntax.KindNamed {
		p.expected("expression")
		mArg = p.errorMarker(mArg)
	}
	// Wrap to join adjacent math content, but not when count == 1 (wrapping
	// would change the type of a non-content expression, e.g. `func(#12pt)`).
	if count != 1 {
		p.wrap(mArg, syntax.KindMath)
	}
	if argKind != syntax.KindInvalid {
		p.wrap(m, argKind)
	}
}

func (p *parser) parseBlock() {
	switch p.cur.kind {
	case syntax.KindLeftBrace:
		p.parseCodeBlock()
	case syntax.KindLeftBracket:
		p.parseContentBlock()
	default:
		p.expected("block")
	}
}

var contentBlockStops = syntax.SetOf(syntax.KindRightBracket, syntax.KindEnd)

func (p *parser) parseContentBlock() {
	start := len(p.nodes)
	p.withMode(syntax.ModeMarkup, nlContinue, func() {
		p.assert(syntax.KindLeftBracket)
		p.parseMarkup(contentBlockStops, mfAtStart|mfWrapTrivia)
		p.expectClosing(start, syntax.KindRightBracket)
	})
	p.wrap(start, syntax.KindContentBlock)
}

var codeBlockStops = syntax.SetOf(syntax.KindRightBrace, syntax.KindRightBracket, syntax.KindRightParen, syntax.KindEnd)

func (p *parser) parseCodeBlock() {
	start := len(p.nodes)
	p.withMode(syntax.ModeCode, nlContinue, func() {
		p.assert(syntax.KindLeftBrace)
		p.parseCode(codeBlockStops)
		p.expectClosing(start, syntax.KindRightBrace)
	})
	p.wrap(start, syntax.KindCodeBlock)
}

func (p *parser) parseCodeExprPrec(atomic bool, minPrec int) {
	if !p.enterDepth(noStops) {
		return
	}

	start := len(p.nodes)
	if !atomic && p.atSet(syntax.UnaryOps) {
		op := syntax.UnaryOpFromKind(p.cur.kind)
		p.consume()
		p.parseCodeExprPrec(atomic, op.Precedence())
		p.wrap(start, syntax.KindUnary)
	} else {
		start = p.parseCodePrimary(atomic)
	}

	for {
		if p.directlyAt(syntax.KindLeftParen) || p.directlyAt(syntax.KindLeftBracket) {
			p.parseArgs()
			p.wrap(start, syntax.KindFuncCall)
			continue
		}

		// In atomic mode, continue with a field access only when an identifier
		// follows. Otherwise the dot may be punctuation in markup, as in "foo."
		// at the end of a sentence.
		if atomic {
			if !p.directlyAt(syntax.KindDot) {
				break
			}
			off := p.s.Offset()
			kind, _ := p.s.Next()
			p.s.Seek(off)
			if kind != syntax.KindIdent {
				break
			}
		}

		if p.consumeIf(syntax.KindDot) {
			p.expect(syntax.KindIdent)
			p.wrap(start, syntax.KindFieldAccess)
			continue
		}

		var op syntax.BinaryOp
		if p.atSet(syntax.BinaryOps) {
			op = syntax.BinaryOpFromKind(p.cur.kind)
		} else if minPrec <= syntax.NotIn.Precedence() && p.at(syntax.KindNot) {
			cp := p.checkpoint()
			p.consume()
			if p.at(syntax.KindIn) {
				op = syntax.NotIn
			} else {
				p.restore(cp)
				p.errorf("expected keyword `in` to follow `not` for `not in` operator")
				break
			}
		} else {
			break
		}

		prec := op.Precedence()
		if prec < minPrec {
			break
		}
		if op.Assoc() == syntax.AssocLeft {
			prec++
		}
		p.consume()
		p.parseCodeExprPrec(false, prec)
		p.wrap(start, syntax.KindBinary)
	}
	p.leaveDepth()
}

func (p *parser) parseCodeExpr() {
	p.parseCodeExprPrec(false, 0)
}

// parseCodePrimary parses a primary code expression and returns the marker at
// which it starts. The marker is returned rather than recomputed by the caller
// because an error-recovery path may have shifted it (see [parser.errorMarker]).
func (p *parser) parseCodePrimary(atomic bool) int {
	start := len(p.nodes)
	switch p.cur.kind {
	case syntax.KindIdent:
		p.consume()
		if !atomic && p.at(syntax.KindArrow) {
			p.parseSingleParamClosure(start)
		}

	case syntax.KindUnderscore:
		if atomic {
			p.errorf("unexpected %s", p.cur.kind.Name())
			break
		}

		p.consume()
		if p.at(syntax.KindArrow) {
			p.parseSingleParamClosure(start)
		} else if p.consumeIf(syntax.KindEq) {
			p.parseCodeExpr()
			p.wrap(start, syntax.KindDestructAssignment)
		} else {
			p.expectedAt(start, "expression")
		}

	case syntax.KindLeftBrace:
		p.parseCodeBlock()

	case syntax.KindLeftBracket:
		p.parseContentBlock()

	case syntax.KindLeftParen:
		p.parseExprWithParen(atomic)

	case syntax.KindDollar:
		p.parseEquation()

	// Literals
	case syntax.KindNone, syntax.KindAuto, syntax.KindBool, syntax.KindInt, syntax.KindFloat,
		syntax.KindNumeric, syntax.KindStr, syntax.KindLabel:
		p.consume()

	case syntax.KindRaw:
		p.consume()

	// Bindings and rules
	case syntax.KindLet:
		p.parseLetBinding()

	case syntax.KindSet:
		p.parseSetRule()

	case syntax.KindShow:
		p.parseShowRule()

	case syntax.KindContext:
		p.parseContextual(atomic)

	// Control flow
	case syntax.KindIf:
		p.parseConditional()

	case syntax.KindWhile:
		p.parseWhileLoop()

	case syntax.KindFor:
		p.parseForLoop()

	case syntax.KindBreak:
		p.parseLoopBreak()

	case syntax.KindContinue:
		p.parseLoopContinue()

	case syntax.KindReturn:
		p.parseFuncReturn()

	// Module operations
	case syntax.KindImport:
		p.parseModuleImport()

	case syntax.KindInclude:
		p.parseModuleInclude()

	default:
		if atomic {
			// Consume erroneous tokens for things like `#12p`, `#]`, or `#"abc\"`.
			p.unexpected()
		} else {
			p.expected("expression")
		}
		start = p.errorMarker(start)
	}
	return start
}

// parseSingleParamClosure wraps the already-consumed parameter as KindParams,
// consumes the arrow, parses the body, and wraps everything as KindClosure.
func (p *parser) parseSingleParamClosure(start int) {
	p.wrap(start, syntax.KindParams)
	p.assert(syntax.KindArrow)
	p.parseCodeExpr()
	p.wrap(start, syntax.KindClosure)
}

// parseExprWithParen parses an expression starting with a '('.
func (p *parser) parseExprWithParen(atomic bool) {
	if atomic {
		// An atomic expression is not changed by whatever follows it, so the
		// first guess of array or dict is always right.
		p.parseParenthesizedOrArrayOrDict()
		return
	}

	// A memoized result for this position means the guess below has already
	// been made and corrected once; restore it rather than repeating the work.
	// Otherwise take a key and a checkpoint to come back to if the guess is
	// wrong.
	if p.restoreMemo(p.s.Offset()) {
		return
	}
	key, cp := p.s.Offset(), p.checkpoint()

	// A '(' can start several things. Try the likely ones first — a
	// parenthesized expression, an array, a dictionary — which one pass
	// handles together.
	kind := p.parseParenthesizedOrArrayOrDict()

	// A following '=>' or '=' means it was a parameter list or a destructuring
	// after all, so restore the checkpoint and parse it again as that.
	//
	// Backtracking on its own would be exponential: in a nest like
	// `(x: (x: (x) => y) => y) => y` every level guesses wrong and reparses
	// the levels inside it, which is O(2^n) for depth n. The memo map stops
	// that. Once a position has been parsed correctly, its result is saved,
	// and reaching that position again restores the result instead of parsing
	// it. No parenthesized expression is parsed more than twice.
	if p.at(syntax.KindArrow) {
		p.restore(cp)
		start := len(p.nodes)
		p.parseParams()
		if !p.expect(syntax.KindArrow) {
			return
		}
		p.parseCodeExpr()
		p.wrap(start, syntax.KindClosure)
	} else if p.at(syntax.KindEq) && kind != syntax.KindParenthesized {
		p.restore(cp)
		start := len(p.nodes)
		p.parseDestructuringOrParenthesized(true, false)
		if !p.expect(syntax.KindEq) {
			return
		}
		p.parseCodeExpr()
		p.wrap(start, syntax.KindDestructAssignment)
	} else {
		return
	}

	p.memoizeNodes(key, cp.nodes)
}

func (p *parser) parseParams() {
	start := len(p.nodes)
	p.withNewlineMode(nlContinue, func() {
		p.assert(syntax.KindLeftParen)

		sink := false
		for !p.atSet(syntax.Terminator) {
			if !p.atSet(syntax.Param) {
				p.unexpected()
				continue
			}

			p.parseParam(&sink)

			if !p.atSet(syntax.Terminator) {
				p.expect(syntax.KindComma)
			}
		}

		p.expectClosing(start, syntax.KindRightParen)
	})
	p.wrap(start, syntax.KindParams)
}

func (p *parser) parseParam(sink *bool) {
	start := len(p.nodes)

	// Argument sink: `...args`.
	if p.consumeIf(syntax.KindDots) {
		if p.atSet(syntax.PatternLeaf) {
			p.parsePatternLeaf(false)
		}
		p.wrap(start, syntax.KindSpread)
		*sink = true
		return
	}

	// Normal positional parameter or a parameter name.
	wasAtPattern := p.atSet(syntax.Pattern)
	p.parsePattern(false)

	// Named parameter: `name: value`.
	if p.consumeIf(syntax.KindColon) {
		if wasAtPattern && p.nodes[start].Kind() != syntax.KindIdent {
			p.expectedAt(start, "identifier")
		}

		p.parseCodeExpr()
		p.wrap(start, syntax.KindNamed)
	}
}

// parsePattern parses a binding or reassignment pattern.
// A reassignment pattern allows expressions; a binding pattern allows only
// identifiers.
func (p *parser) parsePattern(reassignment bool) {
	// Destructuring nests without going through parseCodeExprPrec, so it is
	// counted here as well.
	if !p.enterDepth(noStops) {
		return
	}
	defer p.leaveDepth()

	switch p.cur.kind {
	case syntax.KindUnderscore:
		p.consume()
	case syntax.KindLeftParen:
		// isDestruct is false: a lone parenthesised identifier in a pattern
		// position, such as the key in `((x): v)` or the inside of `((a, b))`,
		// is a grouping rather than a destructure. The caller already wraps
		// real destructures, in parseLetBinding or destructuring-assignment
		// recovery.
		p.parseDestructuringOrParenthesized(reassignment, false)
	default:
		p.parsePatternLeaf(reassignment)
	}
}

// parseDestructuringOrParenthesized parses a destructuring pattern or just a parenthesized pattern.
func (p *parser) parseDestructuringOrParenthesized(reassignment bool, isDestruct bool) {
	var sink bool
	var count int
	notJustParens := isDestruct

	start := len(p.nodes)
	p.withNewlineMode(nlContinue, func() {
		p.assert(syntax.KindLeftParen)

		for !p.atSet(syntax.Terminator) {
			if !p.atSet(syntax.DestructuringItem) {
				p.unexpected()
				continue
			}

			p.parseDestructuringItem(reassignment, &notJustParens, &sink)
			count++

			if !p.atSet(syntax.Terminator) && p.expect(syntax.KindComma) {
				notJustParens = true
			}
		}

		p.expectClosing(start, syntax.KindRightParen)
	})

	if !notJustParens && count == 1 && !sink {
		p.wrap(start, syntax.KindParenthesized)
	} else {
		p.wrap(start, syntax.KindDestructuring)
	}
}

// parseDestructuringItem parses an item in a destructuring pattern.
func (p *parser) parseDestructuringItem(reassignment bool, notJustParens *bool, sink *bool) {
	start := len(p.nodes)

	// Parse destructuring sink: `..rest`.
	if p.consumeIf(syntax.KindDots) {
		if p.atSet(syntax.PatternLeaf) {
			p.parsePatternLeaf(reassignment)
		}
		p.wrap(start, syntax.KindSpread)
		*sink = true
		return
	}

	// Parse a normal positional pattern or a destructuring key.
	wasAtPattern := p.atSet(syntax.Pattern)

	// A full checkpoint, because there may be trivia between the identifier and
	// the colon to skip back over.
	cp := p.checkpoint()
	if !p.consumeIf(syntax.KindIdent) || !p.at(syntax.KindColon) {
		p.restore(cp)
		p.parsePattern(reassignment)
	}

	// Parse named destructuring item.
	if p.consumeIf(syntax.KindColon) {
		// Recover from bad named destructuring.
		if wasAtPattern && p.nodes[start].Kind() != syntax.KindIdent {
			p.expectedAt(start, "identifier")
		}

		p.parsePattern(reassignment)
		p.wrap(start, syntax.KindNamed)
		*notJustParens = true
	}
}

// parsePatternLeaf parses a leaf in a pattern - either an identifier or an
// expression depending on whether it's a binding or reassignment pattern.
func (p *parser) parsePatternLeaf(reassignment bool) {
	if p.atSet(syntax.Keywords) {
		tok := p.cur
		e := p.errorf("expected pattern, found %s", tok.kind.Name())
		e.Hint(fmt.Sprintf("%s is not allowed as an identifier; try `%s_` instead", tok.kind.Name(), p.text(tok.node)))
		return
	} else if !p.atSet(syntax.PatternLeaf) {
		p.expected("pattern")
		return
	}

	start := len(p.nodes)

	// Parse a whole atomic expression even though only an identifier is
	// wanted: that way the error can name the whole expression, rather than
	// picking through its pieces one at a time.
	p.parseCodeExprPrec(true, 0)

	if !reassignment {
		if p.nodes[start].Kind() != syntax.KindIdent {
			p.expectedAt(start, "pattern")
		}
	}
}

type groupState struct {
	count         int
	notJustParens bool
	kind          syntax.Kind
}

// parseParenthesizedOrArrayOrDict parses either
//   - a parenthesized expression: `(1 + 2)`, or
//   - an array: `(1, "hi", 12cm)`, or
//   - a dictionary: `(thickness: 3pt, dash: "solid")`.
func (p *parser) parseParenthesizedOrArrayOrDict() syntax.Kind {
	var state groupState

	start := len(p.nodes)
	p.withNewlineMode(nlContinue, func() {
		p.assert(syntax.KindLeftParen)
		if p.consumeIf(syntax.KindColon) {
			// `(:` always opens a dictionary, so it must not be downgraded to a
			// parenthesized expression below. That would leave the bare `:` as a
			// child of the parenthesized node, as in `(:0)`.
			state.kind = syntax.KindDict
			state.notJustParens = true
		}

		for !p.atSet(syntax.Terminator) {
			if !p.atSet(syntax.ArrayOrDictItem) {
				p.unexpected()
				continue
			}

			p.parseArrayOrDictItem(&state)
			state.count++
			if !p.atSet(syntax.Terminator) && p.expect(syntax.KindComma) {
				state.notJustParens = true
			}
		}

		p.expectClosing(start, syntax.KindRightParen)
	})

	if !state.notJustParens && state.count == 1 {
		state.kind = syntax.KindParenthesized
	} else if state.kind == syntax.KindInvalid {
		state.kind = syntax.KindArray
	}
	p.wrap(start, state.kind)
	return state.kind
}

func (p *parser) parseArrayOrDictItem(state *groupState) {
	start := len(p.nodes)

	if p.consumeIf(syntax.KindDots) {
		// Spread item
		p.parseCodeExpr()
		p.wrap(start, syntax.KindSpread)
		state.notJustParens = true
		return
	}

	p.parseCodeExpr()

	if p.consumeIf(syntax.KindColon) {
		// Named/keyed pair: `name: item` or `"key": item`.
		p.parseCodeExpr()

		node := p.nodes[start]
		pairKind := syntax.KindKeyed
		if node.Kind() == syntax.KindIdent {
			pairKind = syntax.KindNamed
		}

		// Duplicate key detection is handled in the analyzer.
		p.wrap(start, pairKind)
		state.notJustParens = true

		if state.kind == syntax.KindArray {
			// Dictionary syntax inside an array. It is an error, but the
			// analyzer reports it, not the parser.
			return
		} else {
			state.kind = syntax.KindDict
		}
	} else {
		// Regular array item
		if state.kind != syntax.KindDict {
			state.kind = syntax.KindArray
		}
	}
}

// parseArgs parses a function call's argument list: `(12pt, y)`.
func (p *parser) parseArgs() {
	if !p.directlyAt(syntax.KindLeftParen) && !p.directlyAt(syntax.KindLeftBracket) {
		err := p.expected("argument list")
		if p.at(syntax.KindLeftParen) || p.at(syntax.KindLeftBracket) {
			err.Hint("there may not be any spaces between the function name and the argument list")
		}
		return
	}

	start := len(p.nodes)
	if p.at(syntax.KindLeftParen) {
		p.withNewlineMode(nlContinue, func() {
			leftParen := len(p.nodes)
			p.assert(syntax.KindLeftParen)
			for !p.atSet(syntax.Terminator) {
				if !p.atSet(syntax.Arg) {
					p.unexpected()
					continue
				}

				p.parseArg()

				if !p.atSet(syntax.Terminator) {
					p.expect(syntax.KindComma)
				}
			}

			p.expectClosing(leftParen, syntax.KindRightParen)
		})
	}

	for p.directlyAt(syntax.KindLeftBracket) {
		p.parseContentBlock()
	}

	p.wrap(start, syntax.KindArgs)
}

// parseArg parses a single argument in an argument list.
func (p *parser) parseArg() {
	start := len(p.nodes)

	// Spread argument: `...args`.
	if p.consumeIf(syntax.KindDots) {
		p.parseCodeExpr()
		p.wrap(start, syntax.KindSpread)
		return
	}

	// Normal positional argument
	wasAtExpr := p.atSet(syntax.CodeExpr)
	p.parseCodeExpr()

	// Named argument: `name: value`.
	if p.consumeIf(syntax.KindColon) {
		// Recover from bad argument name.
		if wasAtExpr {
			if p.nodes[start].Kind() != syntax.KindIdent {
				p.expectedAt(start, "identifier")
			}
			// Duplicate argument detection is handled in the analyzer.
		}
		p.parseCodeExpr()
		p.wrap(start, syntax.KindNamed)
	}
}

// parseLetBinding parses a let binding: `let x = 1`.
func (p *parser) parseLetBinding() {
	start := len(p.nodes)
	p.assert(syntax.KindLet)

	closureStart := len(p.nodes)
	var closure bool
	var other bool

	if p.consumeIf(syntax.KindIdent) {
		if p.directlyAt(syntax.KindLeftParen) {
			p.parseParams()
			closure = true
		}
	} else {
		p.parsePattern(false)
		other = true
		if p.directlyAt(syntax.KindLeftParen) {
			p.parseParams()
			closure = true
		}
	}

	if closure || other {
		if p.expect(syntax.KindEq) {
			p.parseCodeExpr()
		}
	} else {
		if p.consumeIf(syntax.KindEq) {
			p.parseCodeExpr()
		}
	}
	if closure {
		p.wrap(closureStart, syntax.KindClosure)
	}
	p.wrap(start, syntax.KindLetBinding)
}

// parseSetRule parses a set rule: `set text(...)`.
func (p *parser) parseSetRule() {
	start := len(p.nodes)
	p.assert(syntax.KindSet)

	inner := len(p.nodes)
	if !p.expect(syntax.KindIdent) {
		// The target is an error node, which the field-access wrap below must
		// still contain (see [parser.errorMarker]).
		inner = p.errorMarker(inner)
	}
	for p.consumeIf(syntax.KindDot) {
		p.expect(syntax.KindIdent)
		p.wrap(inner, syntax.KindFieldAccess)
	}

	p.parseArgs()
	if p.consumeIf(syntax.KindIf) {
		p.parseCodeExpr()
	}
	p.wrap(start, syntax.KindSetRule)
}

// parseShow rule parses  a show rule: `show heading: it => emph(it.body)`.
func (p *parser) parseShowRule() {
	start := len(p.nodes)
	p.assert(syntax.KindShow)

	if p.consumeIf(syntax.KindColon) {
		// `show: transform` — no selector.
		p.parseCodeExpr()
	} else {
		p.parseCodeExpr()
		// Index of the selector, taken after parsing it: parseCodeExpr wraps
		// its result into a single node and error recovery may insert nodes, so
		// a marker taken beforehand would point at the wrong node — or past the
		// end. It is the last node before any pending trivia.
		selector := len(p.nodes) - p.cur.trivia - 1
		if p.consumeIf(syntax.KindColon) {
			p.parseCodeExpr()
		} else {
			p.expectedAt(selector, "colon")
		}
	}

	p.wrap(start, syntax.KindShowRule)
}

// parseContextual parses a contextual expression: `context text.lang`.
func (p *parser) parseContextual(atomic bool) {
	start := len(p.nodes)
	p.assert(syntax.KindContext)
	p.parseCodeExprPrec(atomic, 0)
	p.wrap(start, syntax.KindContextual)
}

// parseConditional parses an if-else conditional: `if x { y } else { z }`.
func (p *parser) parseConditional() {
	start := len(p.nodes)
	p.assert(syntax.KindIf)
	p.parseCodeExpr()
	p.parseBlock()
	if p.consumeIf(syntax.KindElse) {
		if p.at(syntax.KindIf) {
			p.parseConditional()
		} else {
			p.parseBlock()
		}
	}
	p.wrap(start, syntax.KindConditional)
}

func (p *parser) parseWhileLoop() {
	start := len(p.nodes)
	p.assert(syntax.KindWhile)
	p.parseCodeExpr()
	p.parseBlock()
	p.wrap(start, syntax.KindWhileLoop)
}

func (p *parser) parseForLoop() {
	start := len(p.nodes)
	p.assert(syntax.KindFor)

	patternStart := len(p.nodes)
	p.parsePattern(false)
	// Check if pattern parsing produced an error
	hasPatternError := false
	for i := patternStart; i < len(p.nodes); i++ {
		if p.nodes[i].Kind() == syntax.KindError {
			hasPatternError = true
			break
		}
	}

	if p.at(syntax.KindComma) {
		err := p.unexpected()
		err.Hint("destructuring patterns must be wrapped in parentheses")
		if p.atSet(syntax.Pattern) {
			p.parsePattern(false)
		}
	}

	// If there was a pattern error, don't also report "expected keyword `in`".
	// Just consume `in` if present and continue.
	if hasPatternError {
		p.consumeIf(syntax.KindIn)
	} else if !p.expect(syntax.KindIn) {
		// Stop at a terminator or an error token, such as an unclosed string, so
		// the errors do not cascade. Otherwise carry on recovering.
		if p.atSet(syntax.Terminator) || p.cur.kind == syntax.KindError {
			p.wrap(start, syntax.KindForLoop)
			return
		}
	}
	p.parseCodeExpr()
	p.parseBlock()
	p.wrap(start, syntax.KindForLoop)
}

func (p *parser) parseLoopBreak() {
	start := len(p.nodes)
	p.assert(syntax.KindBreak)
	p.wrap(start, syntax.KindLoopBreak)
}

func (p *parser) parseLoopContinue() {
	start := len(p.nodes)
	p.assert(syntax.KindContinue)
	p.wrap(start, syntax.KindLoopContinue)
}

func (p *parser) parseFuncReturn() {
	start := len(p.nodes)
	p.assert(syntax.KindReturn)
	if p.atSet(syntax.CodeExpr) {
		p.parseCodeExpr()
	}
	p.wrap(start, syntax.KindFuncReturn)
}

// parseModuleImport parses a module import: `import "utils.typ": a, b, c`.
func (p *parser) parseModuleImport() {
	start := len(p.nodes)
	p.assert(syntax.KindImport)
	p.parseCodeExpr()

	if p.consumeIf(syntax.KindAs) {
		// Allow renaming a full module import.
		// If items are included, both the full module and the items are
		// imported at the same time.
		p.expect(syntax.KindIdent)
	}

	if p.consumeIf(syntax.KindColon) {
		if p.at(syntax.KindLeftParen) {
			p.withNewlineMode(nlContinue, func() {
				leftParen := len(p.nodes)
				p.assert(syntax.KindLeftParen)
				p.parseImportItems()
				p.expectClosing(leftParen, syntax.KindRightParen)
			})
		} else if !p.consumeIf(syntax.KindStar) {
			p.parseImportItems()
		}
	}

	p.wrap(start, syntax.KindModuleImport)
}

// parseImportItems parses items to import from a module: `a, b, c`.
func (p *parser) parseImportItems() {
	start := len(p.nodes)
	for !p.atSet(syntax.Terminator) {
		itemStart := len(p.nodes)
		if !p.consumeIf(syntax.KindIdent) {
			p.unexpected()
		}

		// Nested import path: `a.b.c`
		for p.consumeIf(syntax.KindDot) {
			p.expect(syntax.KindIdent)
		}

		p.wrap(itemStart, syntax.KindImportItemPath)

		// Rename imported item.
		if p.consumeIf(syntax.KindAs) {
			p.expect(syntax.KindIdent)
			p.wrap(itemStart, syntax.KindRenamedImportItem)
		}

		if !p.atSet(syntax.Terminator) {
			p.expect(syntax.KindComma)
		}
	}

	p.wrap(start, syntax.KindImportItems)
}

func (p *parser) parseModuleInclude() {
	start := len(p.nodes)
	p.assert(syntax.KindInclude)
	p.parseCodeExpr()
	p.wrap(start, syntax.KindModuleInclude)
}
