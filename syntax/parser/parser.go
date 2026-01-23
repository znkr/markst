package parser

import (
	"fmt"
	"slices"

	"znkr.io/writst/syntax"
	"znkr.io/writst/syntax/scanner"
)

var stopParse = syntax.SetOf(syntax.KindEnd)

func Parse(src string) syntax.Node {
	p := newParser(src)
	p.parseMarkup(stopParse, mfAtStart|mfWrapTrivia)
	if p.cur.kind != syntax.KindEnd {
		panic("expected end of input")
	}
	if len(p.nodes) != 1 {
		panic("expected single root node")
	}
	return p.nodes[0]
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

	// state
	cur         token
	nodes       []syntax.Node
	newlineMode nlMode
	memos       map[int]memo
}

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

func newParser(src string) *parser {
	p := &parser{
		s:     scanner.New(src),
		memos: make(map[int]memo),
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

func (p *parser) unexpected() *syntax.ErrorValue {
	n := syntax.Error("unexpected", p.cur.node.Value.Text())
	p.nodes = append(p.nodes, n)
	p.next()
	return n.AsError()
}

func (p *parser) expected(expected string) *syntax.ErrorValue {
	at := len(p.nodes) - p.cur.trivia
	if at > 0 && p.nodes[at-1].Kind == syntax.KindError {
		// Already have an error at this position.
		return p.nodes[at-1].AsError()
	}
	n := syntax.Error(fmt.Sprintf("expected %s", expected), p.cur.node.Value.Text())
	p.nodes = slices.Insert(p.nodes, at, n)
	return n.AsError()
}

func (p *parser) expectedAt(i int, expected string) *syntax.ErrorValue {
	prev := p.nodes[i]
	if prev.Kind == syntax.KindError {
		// Already have an error at this position.
		return prev.AsError()
	}
	n := syntax.Error(fmt.Sprintf("expected %s", expected), prev.Value.Text())
	p.nodes[i] = n
	return n.AsError()
}

func (p *parser) consumeAs(kind syntax.Kind) {
	if p.at(syntax.KindError) {
		panic("cannot convert error node")
	}
	p.cur.node.Kind = kind
	p.consume()
}

func (p *parser) assert(expected syntax.Kind) {
	if p.cur.kind != expected {
		panic(fmt.Sprintf("expected %s, got %s", expected, p.cur.node.Kind))
	}
	p.consume()
}

func (p *parser) expect(expected syntax.Kind) bool {
	if p.cur.kind == expected {
		p.consume()
		return true
	}
	n := syntax.Error(fmt.Sprintf("expected %s", expected), p.cur.node.Value.Text())
	p.nodes = append(p.nodes, n)
	return false
}

func (p *parser) expectClosing(open int, expected syntax.Kind) *syntax.ErrorValue {
	if p.cur.kind == expected {
		p.consume()
		return nil
	}
	if p.nodes[open].Kind == syntax.KindError {
		// Already have an error at this position.
		return nil
	}
	n := syntax.Error("unclosed delimiter", p.nodes[open].Value.Text())
	p.nodes[open] = n
	return n.AsError()
}

func (p *parser) flushTrivia() {
	p.cur.trivia = 0
	p.cur.prevEnd = p.cur.start
}

func (p *parser) wrap(start int, kind syntax.Kind) {
	to := len(p.nodes) - p.cur.trivia
	from := min(start, to)
	children := slices.Clone(p.nodes[from:to])
	p.nodes = slices.Delete(p.nodes, from, to)
	p.nodes = slices.Insert(p.nodes, from, syntax.Inner(kind, children))
}

func (p *parser) withMode(mode syntax.Mode, nlmode nlMode, fn func()) {
	prev := p.s.Mode()
	p.s.SetMode(mode)
	p.withNewlineMode(nlmode, fn)
	if mode != prev {
		p.s.SetMode(prev)
		// Rescan last token to make sure we continue with the correct token (scanning might be
		// different in the previous mode).
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
		p.cur.kind = p.cur.node.Kind // restore correct kind
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
	if flags&mfWrapTrivia != 0 {
		start -= p.cur.trivia
	}
	nesting := 0
	atStart := p.cur.newline || flags&mfAtStart != 0
	for !p.atSet(stops) {
		switch p.cur.kind {
		case syntax.KindLeftBracket:
			nesting++
			p.consumeAs(syntax.KindText)
		case syntax.KindRightBracket:
			if nesting > 0 {
				nesting--
				p.consumeAs(syntax.KindText)
			} else {
				err := p.unexpected()
				err.AddHint("try using a backslash escape: \\]")
			}
		case syntax.KindStar:
			p.parseStrong()
		case syntax.KindUnderscore:
			p.parseEmph()
		case syntax.KindHeadingMarker:
			if atStart {
				p.parseHeading()
			} else {
				p.consumeAs(syntax.KindText)
			}
		case syntax.KindListMarker:
			if atStart {
				p.parseListItem()
			} else {
				p.consumeAs(syntax.KindText)
			}
		case syntax.KindEnumMarker:
			if atStart {
				p.parseEnumItem()
			} else {
				p.consumeAs(syntax.KindText)
			}
		case syntax.KindTermMarker:
			if atStart {
				p.parseTermItem()
			} else {
				p.consumeAs(syntax.KindText)
			}
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
	if flags&mfWrapTrivia != 0 {
		p.flushTrivia()
	}
	p.wrap(start, syntax.KindMarkup)
}

func (p *parser) parseCode(stops syntax.Set) {
	start := len(p.nodes) - p.cur.trivia
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
					err.AddHint("labels can only be applied in markup mode")
					err.AddHint("try wrapping your code in a markup block (`[ ]`)")
				}
			}
		})
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
		at := p.atSet(syntax.AtomicCodeExpr)
		p.parseCodeExprPrec(true, 0)

		// Consume error for things like `#12p` or `#"abc\"`.
		if !at {
			p.unexpected()
		}

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

func (p *parser) parseEquation() {
	panic("equation parsing not implemented yet")
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
	start := len(p.nodes)
	if !atomic && p.atSet(syntax.UnaryOps) {
		op := syntax.UnaryOpFromKind(p.cur.kind)
		p.consume()
		p.parseCodeExprPrec(atomic, op.Precedence())
		p.wrap(start, syntax.KindUnary)
	} else {
		p.parseCodePrimary(atomic)
	}

	for {
		if p.directlyAt(syntax.KindLeftParen) || p.directlyAt(syntax.KindLeftBracket) {
			p.parseArgs()
			p.wrap(start, syntax.KindFuncCall)
			continue
		}

		// In atomic mode, only continue with field access if followed by an identifier. Otherwise,
		// the dot might be punctuation in markup mode (e.g., "foo." at end of sentence).
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
		} else if minPrec <= syntax.NotIn.Precedence() && p.consumeIf(syntax.KindNot) {
			if p.at(syntax.KindIn) {
				op = syntax.NotIn
			} else {
				p.expected("keyword `in`")
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
}

func (p *parser) parseCodeExpr() {
	p.parseCodeExprPrec(false, 0)
}

func (p *parser) parseCodePrimary(atomic bool) {
	start := len(p.nodes)
	switch p.cur.kind {
	case syntax.KindIdent:
		p.consume()
		if !atomic && p.at(syntax.KindArrow) {
			p.wrap(start, syntax.KindParams)
			p.assert(syntax.KindArrow)
			p.parseCodeExpr()
			p.wrap(start, syntax.KindClosure)
		}

	case syntax.KindUnderscore:
		p.consume()
		if !atomic && p.at(syntax.KindArrow) {
			p.wrap(start, syntax.KindParams)
			p.assert(syntax.KindArrow)
			p.parseCodeExpr()
			p.wrap(start, syntax.KindClosure)
		} else if !atomic && p.consumeIf(syntax.KindEq) {
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
		p.expected("expression")
	}
}

// parseExprWithParen parses an expression starting with a '('.
func (p *parser) parseExprWithParen(atomic bool) {
	if atomic {
		// Atomic expressions aren't modified by operators that follow them, so our first guess of
		// array/dict will be correct.
		p.parseParenthesizedOrArrayOrDict()
		return
	}

	// If we've seen this position before and have a memoized result, restore it and return.
	// Otherwise, get a key to this position and a checkpoint to restart from in case we make a
	// wrong prediction.
	if p.restoreMemo(p.s.Offset()) {
		return
	}
	key, cp := p.s.Offset(), p.checkpoint()

	// When we reach a '(', we can't be sure what it is. First, we attempt to parse as a simple
	// parenthesized expression, array, or dictionary as these are the most likely things. We can
	// handle all of those in a single pass.
	kind := p.parseParenthesizedOrArrayOrDict()

	// If, however, '=>' or '=' follows, we must backtrack and reparse as either a parameter list or
	// a destructuring. To be able to do that, we created a parser checkpoint before our speculative
	// parse, which we can restore.
	//
	// However, naive backtracking has a fatal flaw: It can lead to exponential parsing time if we
	// are constantly getting things wrong in a nested scenario. The particular failure case for
	// parameter parsing is the following: `(x: (x: (x) => y) => y) => y`
	//
	// Such a structure will reparse over and over again recursively, leading to a running time of
	// O(2^n) for nesting depth n. To prevent this, we perform a simple trick: When we have done the
	// mistake of picking the wrong path once and have subsequently parsed correctly, we save the
	// result of that correct parsing in the `p.memo` map. When we reach the same position again, we
	// can then just restore this result. In this way, no parenthesized expression is parsed more
	// than twice, leading to a worst case running time of O(2n).
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
		p.parseDestructuringOrParenthesized(true)
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
		if *sink {
			p.expectedAt(start, "only one arguments sink is allowed")
		}
		*sink = true
		return
	}

	// Normal positional parameter or a parameter name.
	wasAtPattern := p.atSet(syntax.Pattern)
	p.parsePattern(false)

	// Named parameter: `name: value`.
	if p.consumeIf(syntax.KindColon) {
		if wasAtPattern && p.nodes[start].Kind != syntax.KindIdent {
			p.expectedAt(start, "identifier")
		}

		p.parseCodeExpr()
		p.wrap(start, syntax.KindNamed)
	}
}

// parsePattern parses a binding or reassignment pattern.
// If reassignment is true, we're parsing a reassignment pattern and expressions are allowed.
// If reassignment is false, we're parsing a binding pattern and only identifiers are allowed.
func (p *parser) parsePattern(reassignment bool) {
	switch p.cur.kind {
	case syntax.KindUnderscore:
		p.consume()
	case syntax.KindLeftParen:
		p.parseDestructuringOrParenthesized(reassignment)
	default:
		p.parsePatternLeaf(reassignment)
	}
}

// parseDestructuringOrParenthesized parses a destructuring pattern or just a parenthesized pattern.
func (p *parser) parseDestructuringOrParenthesized(reassignment bool) {
	var sink bool
	var count int
	var notJustParens bool

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
		if *sink {
			p.expectedAt(start, "only one destructuring sink is allowed")
		}
		*sink = true
		return
	}

	// Parse a normal positional pattern or a destructuring key.
	wasAtPattern := p.atSet(syntax.Pattern)

	// Use a full checkpoint because there may be trivia between the identifier
	// and the colon that we need to skip over when backtracking.
	cp := p.checkpoint()
	if !p.consumeIf(syntax.KindIdent) || !p.at(syntax.KindColon) {
		p.restore(cp)
		p.parsePattern(reassignment)
	}

	// Parse named destructuring item.
	if p.consumeIf(syntax.KindColon) {
		// Recover from bad named destructuring.
		if wasAtPattern && p.nodes[start].Kind != syntax.KindIdent {
			p.expectedAt(start, "identifier")
		}

		p.parsePattern(reassignment)
		p.wrap(start, syntax.KindNamed)
		*notJustParens = true
	}
}

// parsePatternLeaf parses a leaf in a pattern - either an identifier or an expression
// depending on whether it's a binding or reassignment pattern.
func (p *parser) parsePatternLeaf(reassignment bool) {
	if p.atSet(syntax.Keywords) {
		p.expected("pattern")
		return
	} else if !p.atSet(syntax.PatternLeaf) {
		p.expected("pattern")
		return
	}

	start := len(p.nodes)

	// We parse an atomic expression even though we only want an identifier for
	// better error recovery. We can mark the whole expression as unexpected
	// instead of going through its pieces one by one.
	p.parseCodeExprPrec(true, 0)

	if !reassignment {
		node := p.nodes[start]
		if node.Kind != syntax.KindIdent {
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
			state.kind = syntax.KindDict
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
		if node.Kind == syntax.KindIdent {
			pairKind = syntax.KindNamed
		}

		// TODO: the official implementation checks for duplicate keys here. This is probably
		// better suited for a later semantic analysis pass.
		p.wrap(start, pairKind)
		state.notJustParens = true

		if state.kind == syntax.KindArray {
			p.expectedAt(start, "expression")
		} else {
			state.kind = syntax.KindDict
		}
	} else {
		// Regular array item
		if state.kind == syntax.KindDict {
			p.expectedAt(start, "named or keyed pair")
		} else {
			state.kind = syntax.KindArray
		}
	}
}

// parseArgs parses a function call's argument list: `(12pt, y)`.
func (p *parser) parseArgs() {
	if !p.directlyAt(syntax.KindLeftParen) && !p.directlyAt(syntax.KindLeftBracket) {
		err := p.expected("argument list")
		if p.at(syntax.KindLeftParen) || p.at(syntax.KindLeftBracket) {
			err.AddHint("there may not be any spaces between the function name and the argument list")
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
			if p.nodes[start].Kind != syntax.KindIdent {
				p.expectedAt(start, "identifier")
			}
			// TODO: the official implementation checks for duplicate names here. This is probably
			// better suited for a later semantic analysis pass.
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
	p.expect(syntax.KindIdent)
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

	inner := len(p.nodes) - p.cur.trivia

	if !p.at(syntax.KindColon) {
		p.parseCodeExpr()
	}

	if p.consumeIf(syntax.KindColon) {
		p.parseCodeExpr()
	} else {
		p.expectedAt(inner, "colon")
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

	p.parsePattern(false)

	if p.at(syntax.KindComma) {
		err := p.unexpected()
		err.AddHint("destructuring patterns must be wrapped in parentheses")
		if p.atSet(syntax.Pattern) {
			p.parsePattern(false)
		}
	}

	p.expect(syntax.KindIn)
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
