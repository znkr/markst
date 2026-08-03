package analyzer

import (
	"znkr.io/writst/builtin"
	"znkr.io/writst/expr"
	"znkr.io/writst/name"
	"znkr.io/writst/syntax"
	"znkr.io/writst/value"
)

// lowerEquation lowers a `$...$` equation. An equation is a "block" equation
// (displayed on its own line) when there is whitespace directly after the
// opening `$` and directly before the closing `$`.
func (a *analyzer) lowerEquation(n syntax.Node) expr.Ref {
	block := equationIsBlock(n)
	ns := a.inner(n, syntax.KindEquation)
	ns.take(syntax.KindDollar)
	var body expr.Ref
	if ns.at(syntax.KindMath) {
		// Install the math module as a low-precedence fallback so math
		// identifiers (its elements first, then symbols like pi) resolve
		// through the normal scope machinery while still being shadowed by any
		// local binding.
		a.openScope()
		a.scope.mathScope = builtin.Math.Def
		a.mathDepth++
		body = a.lowerMathContent(ns.node())
		a.mathDepth--
		a.closeScope()
	} else {
		body = a.b.Const(n.Span(), value.None{})
	}
	return a.b.Equation(n.Span(), block, body)
}

// equationIsBlock reports whether the equation node's delimiters are padded
// with whitespace on both sides (the Typst rule for display equations).
func equationIsBlock(n syntax.Node) bool {
	inner, ok := n.(*syntax.Inner)
	if !ok {
		return false
	}
	ch := inner.Children()
	if len(ch) < 3 {
		return false
	}
	leading := ch[1].Kind() == syntax.KindSpace
	trailing := ch[len(ch)-2].Kind() == syntax.KindSpace
	return leading && trailing
}

// lowerMathContent lowers the contents of a Math node into a single content
// Ref, joining multiple items with a ContentResult.
func (a *analyzer) lowerMathContent(n syntax.Node) expr.Ref {
	ns := a.inner(n, syntax.KindMath)
	var items []expr.Ref
	for child := range ns.all() {
		switch child.Kind() {
		// Parentheses left by `mathUnparen` are invisible grouping markers.
		case syntax.KindLeftParen, syntax.KindRightParen:
			continue
		}
		if ref := a.lowerExpr(child); ref != expr.NoRef {
			items = append(items, ref)
		}
	}
	switch len(items) {
	case 0:
		return a.b.Const(n.Span(), value.None{})
	case 1:
		return items[0]
	default:
		return a.b.MathContentResult(n.Span(), items)
	}
}

// lowerMathOperand lowers a single math operand from the cursor. A leading `#`
// introducing an embedded code expression is skipped by the cursor itself (see
// [skipped]), so the operand is just the node under it.
func (a *analyzer) lowerMathOperand(ns *nodes) expr.Ref {
	return a.lowerExpr(ns.node())
}

func (a *analyzer) lowerMathText(n syntax.Node) expr.Ref {
	return a.b.Const(n.Span(), &value.MathText{Text: a.leaf(n, syntax.KindMathText)})
}

// lowerMathIdent resolves a math identifier through the normal scope chain,
// which inside an equation includes the math module as a low-precedence
// fallback (see [analyzer.lowerEquation]). A resolvable name (a local `let`, a
// Universe binding, or a math symbol/element) becomes that value; an
// unresolved name renders as upright text.
func (a *analyzer) lowerMathIdent(n syntax.Node) expr.Ref {
	ident := a.leaf(n, syntax.KindMathIdent)
	source := name.Make(ident)
	if _, ok := a.lookup(source); ok {
		return a.resolveName(source, n.Span())
	}
	return a.b.Const(n.Span(), &value.MathText{Text: ident})
}

// lowerMathAttach lowers a base with sub-/superscripts and/or primes (for
// example a_1^2, or a base followed by primes). Primes wrap the base directly;
// if there is neither a superscript nor a subscript the (possibly primed) base
// is returned as-is.
func (a *analyzer) lowerMathAttach(n syntax.Node) expr.Ref {
	ns := a.inner(n, syntax.KindMathAttach)
	base := a.lowerMathOperand(ns)
	top, bottom := expr.NoRef, expr.NoRef
	for !ns.done() {
		op := ns.node()
		switch op.Kind() {
		case syntax.KindUnderscore:
			if !ns.done() {
				bottom = a.lowerMathOperand(ns)
			}
		case syntax.KindHat:
			if !ns.done() {
				top = a.lowerMathOperand(ns)
			}
		case syntax.KindMathPrimes:
			base = a.b.MathPrimes(op.Span(), base, primeCount(op.Text()))
		default:
			a.unexpected(op)
		}
	}
	if top == expr.NoRef && bottom == expr.NoRef {
		return base
	}
	return a.b.MathAttach(n.Span(), base, top, bottom)
}

func (a *analyzer) lowerMathFrac(n syntax.Node) expr.Ref {
	ns := a.inner(n, syntax.KindMathFrac)
	num := a.lowerMathOperand(ns)
	ns.take(syntax.KindSlash)
	denom := a.lowerMathOperand(ns)
	return a.b.MathFrac(n.Span(), num, denom)
}

func (a *analyzer) lowerMathRoot(n syntax.Node) expr.Ref {
	ns := a.inner(n, syntax.KindMathRoot)
	ns.take(syntax.KindRoot)
	radicand := a.lowerMathOperand(ns)
	return a.b.MathRoot(n.Span(), expr.NoRef, radicand)
}

// lowerMathPrimes lowers a standalone primes node (with no base) by attaching
// the primes to empty content.
func (a *analyzer) lowerMathPrimes(n syntax.Node) expr.Ref {
	base := a.b.Const(n.Span(), &value.MathText{Text: ""})
	return a.b.MathPrimes(n.Span(), base, primeCount(a.leaf(n, syntax.KindMathPrimes)))
}

func (a *analyzer) lowerMathDelimited(n syntax.Node) expr.Ref {
	ns := a.inner(n, syntax.KindMathDelimited)
	open := a.lowerMathDelim(ns.node())
	body := a.lowerMathContent(ns.node())
	closing := a.lowerMathDelim(ns.node())
	return a.b.MathDelimited(n.Span(), open, body, closing)
}

// lowerMathDelim lowers a delimiter token (MathText or MathShorthand) to a
// MathText content value, mapping shorthand delimiters (`[|`, `|]`) to glyphs.
func (a *analyzer) lowerMathDelim(n syntax.Node) expr.Ref {
	return a.b.Const(n.Span(), &value.MathText{Text: mathShorthand(n.Text())})
}

func (a *analyzer) lowerMathCall(n syntax.Node) expr.Ref {
	ns := a.inner(n, syntax.KindMathCall)
	calleeNode := ns.node()
	argsNode := ns.node()
	// `target.field(args)` is a method call and resolves its callee with
	// method-call error semantics (dictionary keys are not directly callable; a
	// missing member on a content element reports as a missing method), flavored
	// for math mode.
	if calleeNode.Kind() == syntax.KindFieldAccess {
		return a.lowerMathMethodCall(n, calleeNode, argsNode)
	}
	callee := expr.Callee{Ref: a.lowerExpr(calleeNode), Span: calleeNode.Span()}
	args := a.lowerMathArgs(argsNode)
	// vec/mat/cases lay each argument out as its own row/column and ignore
	// linebreaks within a cell, so warn (per cell, at the cell's span) when one
	// is present. Done here rather than at eval time because only the analyzer
	// has the cell spans.
	if word, ok := a.mathCellWord(calleeNode); ok {
		a.warnMathCellLinebreaks(argsNode, word)
	}
	return a.b.Call(n.Span(), callee, args, nil, false, nil)
}

// mathCellWord returns the noun vec/mat/cases use in their "linebreaks are
// ignored in …" warning ("elements" for vec/cases, "cells" for mat), and
// whether calleeNode resolves to one of those math builtins (and isn't
// shadowed by a local binding).
func (a *analyzer) mathCellWord(calleeNode syntax.Node) (string, bool) {
	if calleeNode.Kind() != syntax.KindMathIdent {
		return "", false
	}
	b, ok := a.lookup(name.Make(calleeNode.Text()))
	if !ok {
		return "", false
	}
	vb, ok := b.(valueBinding)
	if !ok {
		return "", false
	}
	switch vb.val {
	case builtin.Vec, builtin.Cases:
		return "elements", true
	case builtin.Mat:
		return "cells", true
	}
	return "", false
}

// warnMathCellLinebreaks emits a "linebreaks are ignored" warning for each
// argument cell that contains a linebreak. Cells are the comma/semicolon-
// separated groups of a math argument list; the warning points at the cell.
func (a *analyzer) warnMathCellLinebreaks(argsNode syntax.Node, word string) {
	inner, ok := argsNode.(*syntax.Inner)
	if !ok {
		return
	}
	var cell []syntax.Node
	flush := func() {
		if len(cell) == 0 {
			return
		}
		if cellHasLinebreak(cell) {
			span := syntax.Span{Start: cell[0].Span().Start, End: cell[len(cell)-1].Span().End}
			a.b.Warn(span, "linebreaks are ignored in "+word, "use commas instead to separate each line")
		}
		cell = cell[:0]
	}
	for _, c := range inner.Children() {
		switch c.Kind() {
		case syntax.KindLeftParen, syntax.KindRightParen, syntax.KindSpace:
			continue
		case syntax.KindComma, syntax.KindSemicolon:
			flush()
		default:
			cell = append(cell, c)
		}
	}
	flush()
}

// cellHasLinebreak reports whether a cell has a linebreak at its own level. A
// multi-item cell's run is wrapped in a single [syntax.KindMath] node, so its
// direct children are inspected; linebreaks nested inside an item (an
// attachment's script, a parenthesized group, …) are laid out normally and
// don't count.
func cellHasLinebreak(cell []syntax.Node) bool {
	for _, n := range cell {
		if n.Kind() == syntax.KindLinebreak {
			return true
		}
		if n.Kind() == syntax.KindMath {
			if inner, ok := n.(*syntax.Inner); ok {
				for _, c := range inner.Children() {
					if c.Kind() == syntax.KindLinebreak {
						return true
					}
				}
			}
		}
	}
	return false
}

// lowerMathMethodCall lowers a math-mode field call `target.field(args)`. The
// callee resolves through a math-flavored [Builder.MethodField]; the attached
// [expr.MutCheck] carries the Math flag so that a resolved mutating method is
// rejected at runtime ("cannot call mutating methods in math") — math has no
// mutable place to write the result back to.
func (a *analyzer) lowerMathMethodCall(callNode, faNode, argsNode syntax.Node) expr.Ref {
	fns := a.inner(faNode, syntax.KindFieldAccess)
	targetNode := fns.node()
	fns.take(syntax.KindDot)
	fieldNode := fns.node()
	method := name.Make(a.leaf(fieldNode, syntax.KindIdent))

	targetRef := a.lowerExpr(targetNode)
	calleeRef := a.b.MethodField(faNode.Span(), fieldNode.Span(), targetRef, method, targetNode.Text(), true)
	callee := expr.Callee{Ref: calleeRef, Span: faNode.Span()}
	args := a.lowerMathArgs(argsNode)

	mut := &expr.MutCheck{RecvSpan: callNode.Span(), Math: true, MathCall: callNode.Text()}
	return a.b.Call(callNode.Span(), callee, args, nil, false, mut)
}

// lowerMathArgs lowers a math argument list. When the list contains row
// separators (`;`), the positional cells between semicolons are grouped into
// array arguments (one per row) so that 2D elements like `mat` can recover the
// row structure; column separators (`,`) always separate cells. Without any
// `;`, positional cells are passed flat. Named and spread arguments are passed
// through unchanged.
func (a *analyzer) lowerMathArgs(n syntax.Node) []expr.CallArg {
	twoD := mathArgsHaveSemicolon(n)
	ns := a.inner(n, syntax.KindMathArgs)
	var args []expr.CallArg
	var row []expr.ArrayItem
	seen := make(map[string]struct{})
	// An empty cell is held back until something follows it, so that a trailing
	// one — what the parser leaves between two separators in `mat(a,;b)` or
	// `mat(a;,)` — is dropped as the separator noise it is. That keeps
	// `mat(..nums)`, `mat(..nums;)`, `mat(..nums;,)` and `mat(..nums,)` the same
	// matrix. An empty cell that is followed by another one is a real cell:
	// `mat(1,,2)` has three columns.
	var pending syntax.Node
	flushRow := func() {
		pending = nil
		if len(row) == 0 {
			return
		}
		arr := a.b.MakeArray(n.Span(), row)
		args = append(args, expr.CallArg{Kind: expr.ArgPositional, Value: arr, Span: n.Span()})
		row = nil
	}
	lowerCell := func(cell syntax.Node) {
		ref := a.lowerExpr(cell)
		if twoD {
			row = append(row, expr.ArrayItem{Value: ref, Span: cell.Span()})
		} else {
			args = append(args, expr.CallArg{Kind: expr.ArgPositional, Value: ref, Span: cell.Span()})
		}
	}
	if ns.at(syntax.KindLeftParen) || ns.at(syntax.KindError) {
		for child := range ns.inside(syntax.KindLeftParen, syntax.KindRightParen) {
			switch child.Kind() {
			case syntax.KindComma:
				continue
			case syntax.KindSemicolon:
				flushRow()
				continue
			case syntax.KindSpread:
				flushRow()
				args = append(args, expr.CallArg{Kind: expr.ArgSpread, Value: a.lowerSpread(child), Span: child.Span()})
			case syntax.KindNamed:
				flushRow()
				named := a.inner(child, syntax.KindNamed)
				keyNode := named.node()
				key := name.Make(a.leaf(keyNode, syntax.KindIdent))
				named.take(syntax.KindColon)
				valNode := named.node()
				val := a.lowerExpr(valNode)
				if _, dup := seen[key.String()]; dup {
					a.emitError(keyNode.Span(), "duplicate argument: "+key.String())
				}
				seen[key.String()] = struct{}{}
				args = append(args, expr.CallArg{Kind: expr.ArgNamed, Name: key, Value: val, Span: valNode.Span(), PairSpan: child.Span()})
			default:
				if isEmptyMathCell(child) {
					pending = child
					continue
				}
				if pending != nil {
					cell := pending
					pending = nil
					lowerCell(cell)
				}
				lowerCell(child)
			}
		}
		flushRow()
	}
	return args
}

// isEmptyMathCell reports whether a cell node carries nothing. The parser leaves
// such a node between two adjacent separators (see [analyzer.lowerMathArgs]).
func isEmptyMathCell(n syntax.Node) bool {
	if n.Kind() != syntax.KindMath {
		return false
	}
	inner, ok := n.(*syntax.Inner)
	if !ok {
		return false
	}
	for _, c := range inner.Children() {
		if c.Kind() != syntax.KindSpace {
			return false
		}
	}
	return true
}

// mathArgsHaveSemicolon reports whether a MathArgs node contains a `;` row
// separator.
func mathArgsHaveSemicolon(n syntax.Node) bool {
	inner, ok := n.(*syntax.Inner)
	if !ok {
		return false
	}
	for _, c := range inner.Children() {
		if c.Kind() == syntax.KindSemicolon {
			return true
		}
	}
	return false
}

// primeCount counts the prime marks in a MathPrimes token (all ASCII `'`).
func primeCount(text string) int {
	return len(text)
}

// mathShorthands maps math-mode shorthands to their Unicode codepoints,
// mirroring Typst's math shorthand table.
var mathShorthands = map[string]string{
	"...":  "…", // …
	"-":    "−", // −
	"*":    "∗", // ∗
	"~":    "∼", // ∼
	"!=":   "≠", // ≠
	":=":   "≔", // ≔
	"::=":  "⩴", // ⩴
	"=:":   "≕", // ≕
	"<<":   "≪", // ≪
	"<<<":  "⋘", // ⋘
	">>":   "≫", // ≫
	">>>":  "⋙", // ⋙
	"<=":   "≤", // ≤
	">=":   "≥", // ≥
	"->":   "→", // →
	"-->":  "⟶", // ⟶
	"|->":  "↦", // ↦
	">->":  "↣", // ↣
	"->>":  "↠", // ↠
	"<-":   "←", // ←
	"<--":  "⟵", // ⟵
	"<-<":  "↢", // ↢
	"<<-":  "↞", // ↞
	"<->":  "↔", // ↔
	"<-->": "⟷", // ⟷
	"~>":   "⇝", // ⇝
	"~~>":  "⟿", // ⟿
	"<~":   "⇜", // ⇜
	"<~~":  "⬳", // ⬳
	"=>":   "⇒", // ⇒
	"|=>":  "⤇", // ⤇
	"==>":  "⟹", // ⟹
	"<==":  "⟸", // ⟸
	"<=>":  "⇔", // ⇔
	"<==>": "⟺", // ⟺
	"[|":   "⟦", // ⟦
	"|]":   "⟧", // ⟧
	"||":   "‖", // ‖
}

// mathShorthand returns the Unicode glyph for a math shorthand, or the original
// text if the shorthand is unknown.
func mathShorthand(text string) string {
	if glyph, ok := mathShorthands[text]; ok {
		return glyph
	}
	return text
}
