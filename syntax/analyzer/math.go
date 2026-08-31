package analyzer

import (
	"strings"

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
		// Empty content, not `none`: a math run is content even when it holds
		// nothing, which is observable for an empty argument cell.
		return a.b.Const(n.Span(), &value.Sequence{})
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

// lowerMathIdent resolves a math identifier against what math can see — a
// local `let` or a math symbol/element, but not the universe (see
// [analyzer.lookupMath]). A name math does not have is an error: writing
// several letters in a row means the variable of that name, and `$a b$` is how
// the letters themselves are written.
func (a *analyzer) lowerMathIdent(n syntax.Node) expr.Ref {
	ident := a.leaf(n, syntax.KindMathIdent)
	source := name.Make(ident)
	if _, ok := a.lookupMath(source); ok {
		return a.resolveMathName(source, n.Span())
	}
	return a.emitError(n.Span(), "unknown variable: "+ident, unknownMathVarHints(a, source, ident)...)
}

// unknownMathVarHints suggests what an unknown math identifier might have meant.
// A name the universe does have was reached for in the wrong mode; anything
// else is most likely a run of letters that wants spaces or quotes.
func unknownMathVarHints(a *analyzer, source name.Name, ident string) []string {
	if _, ok := a.lookup(source); ok {
		return []string{
			"`" + ident + "` is not available directly in math, but is in the standard library",
			"to access `" + ident + "` in code mode you can add a hash: `#" + ident + "`",
			"or access `" + ident + "` in math mode by using the `std` module: `std." + ident + "`",
		}
	}
	spaced := strings.Join(strings.Split(ident, ""), " ")
	return []string{
		"if you meant to display multiple letters as is, try adding spaces between each letter: `" + spaced + "`",
		"or if you meant to display this as text, try placing it in quotes: `\"" + ident + "\"`",
	}
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
	args, fallback := a.lowerMathArgs(argsNode, calleeNode)
	if a.mathCalleeIsFunc(calleeNode) {
		fallback = nil
	}
	// vec/mat/cases lay each argument out as its own row/column and ignore
	// linebreaks within a cell, so warn (per cell, at the cell's span) when one
	// is present. Done here rather than at eval time because only the analyzer
	// has the cell spans.
	if word, ok := a.mathCellWord(calleeNode); ok {
		a.warnMathCellLinebreaks(argsNode, word)
	}
	return a.b.MathCall(n.Span(), callee, args, nil, fallback)
}

// mathCalleeIsFunc reports whether a math call's callee is known at analysis
// time to be callable. When it isn't — an unresolved name, a constant that
// holds content, or a variable whose value is only known at runtime — the call
// carries a [expr.MathFallback] so the evaluator can render it as
// juxtaposition instead.
func (a *analyzer) mathCalleeIsFunc(calleeNode syntax.Node) bool {
	if calleeNode.Kind() != syntax.KindMathIdent {
		return false
	}
	b, ok := a.lookupMath(name.Make(calleeNode.Text()))
	if !ok {
		return false
	}
	vb, ok := b.(valueBinding)
	if !ok {
		return false
	}
	// Mirrors the runtime resolution in eval's calleeFunc.
	switch v := vb.val.(type) {
	case *value.Function:
		return true
	case *value.Type:
		return v.Constructor != nil
	case *value.Element:
		return v.F != nil
	case *value.Symbol:
		_, err := builtin.SymbolFunc(v)
		return err == nil
	}
	return false
}

// mathCellWord returns the noun vec/mat/cases use in their "linebreaks are
// ignored in …" warning ("elements" for vec/cases, "cells" for mat), and
// whether calleeNode resolves to one of those math builtins (and isn't
// shadowed by a local binding).
func (a *analyzer) mathCellWord(calleeNode syntax.Node) (string, bool) {
	if calleeNode.Kind() != syntax.KindMathIdent {
		return "", false
	}
	b, ok := a.lookupMath(name.Make(calleeNode.Text()))
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
	args, fallback := a.lowerMathArgs(argsNode, faNode)

	mut := &expr.MutCheck{RecvSpan: callNode.Span(), Math: true, MathCall: callNode.Text()}
	return a.b.MathCall(callNode.Span(), callee, args, mut, fallback)
}

// lowerMathArgs lowers a math argument list. When the list contains row
// separators (`;`), the positional cells between semicolons are grouped into
// array arguments (one per row) so that 2D elements like `mat` can recover the
// row structure; column separators (`,`) always separate cells. Without any
// `;`, positional cells are passed flat. Named and spread arguments are passed
// through unchanged.
//
// The second result is the same list read as plain content — the cells and
// separators in source order — for the case where the callee named by
// calleeNode turns out not to be a function (see [expr.MathFallback]).
func (a *analyzer) lowerMathArgs(n syntax.Node, calleeNode syntax.Node) ([]expr.CallArg, *expr.MathFallback) {
	twoD := mathArgsHaveSemicolon(n)
	ns := a.inner(n, syntax.KindMathArgs)
	var args []expr.CallArg
	var row []expr.ArrayItem
	fallback := &expr.MathFallback{}
	seen := make(map[string]struct{})
	// A `;` ends a row even when nothing was written in it, so `f(a: b;)` has
	// one empty row. The implicit end of the list does not: a trailing `;`
	// closes the last row rather than opening another.
	emitRow := func() {
		arr := a.b.MakeArray(n.Span(), row)
		args = append(args, expr.CallArg{Kind: expr.ArgPositional, Value: arr, Span: n.Span()})
		row = nil
	}
	flushRow := func() {
		if len(row) > 0 {
			emitRow()
		}
	}
	lowerCell := func(cell syntax.Node) {
		ref := a.lowerExpr(cell)
		fallback.Items = append(fallback.Items, expr.MathItem{Ref: ref})
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
				fallback.Items = append(fallback.Items, expr.MathItem{Ref: expr.NoRef, Text: ","})
				continue
			case syntax.KindSemicolon:
				fallback.Items = append(fallback.Items, expr.MathItem{Ref: expr.NoRef, Text: ";"})
				emitRow()
				continue
			case syntax.KindSpread:
				fallback.BadArgs = append(fallback.BadArgs, badMathArg(child, calleeNode, "spread"))
				ref := a.lowerSpread(child)
				if twoD {
					// A spread inside a row list contributes its elements to the
					// row it stands in, not a row of its own.
					row = append(row, expr.ArrayItem{Value: ref, Span: child.Span(), Spread: true})
					continue
				}
				args = append(args, expr.CallArg{Kind: expr.ArgSpread, Value: ref, Span: child.Span()})
			case syntax.KindNamed:
				named := a.inner(child, syntax.KindNamed)
				keyNode := named.node()
				named.take(syntax.KindColon)
				valNode := named.node()
				val := a.lowerExpr(valNode)
				if e, ok := keyNode.(*syntax.Error); ok {
					// The name didn't parse — a duplicate, or not an identifier at
					// all. Pass the poison value in the argument's place so the
					// call short-circuits on it rather than reporting a second
					// error about an argument that is neither named nor missing.
					poison := a.emitSyntaxError(e)
					if twoD {
						row = append(row, expr.ArrayItem{Value: poison, Span: child.Span()})
					} else {
						args = append(args, expr.CallArg{Kind: expr.ArgPositional, Value: poison, Span: child.Span()})
					}
					continue
				}
				fallback.BadArgs = append(fallback.BadArgs, badMathArg(child, calleeNode, "named"))
				key := name.Make(a.leaf(keyNode, syntax.KindIdent))
				if _, dup := seen[key.String()]; dup {
					a.emitError(keyNode.Span(), "duplicate argument: "+key.String())
				}
				seen[key.String()] = struct{}{}
				args = append(args, expr.CallArg{Kind: expr.ArgNamed, Name: key, Value: val, Span: valNode.Span(), PairSpan: child.Span()})
			default:
				lowerCell(child)
			}
		}
		flushRow()
	}
	return args, fallback
}

// badMathArg builds the diagnostic for a named or spread argument (kind is
// "named" or "spread") of a math call whose callee is not a function. Such an
// argument means nothing in the juxtaposition form the call falls back to, so
// the hint says how to write it as plain text instead.
func badMathArg(arg, calleeNode syntax.Node, kind string) expr.MathBadArg {
	hint := "to render the dots as text, add a space: `" + strings.Replace(arg.Text(), "..", ".. ", 1) + "`"
	if kind == "named" {
		hint = "to render the colon as text, escape it: `" + strings.Replace(arg.Text(), ":", `\:`, 1) + "`"
	}
	return expr.MathBadArg{
		Span: arg.Span(),
		Msg:  kind + "-argument syntax can only be used with functions",
		Hints: []value.Hint{
			{Span: calleeNode.Span(), Msg: "`" + calleeNode.Text() + "` is not a function"},
			{Span: syntax.NoSpan, Msg: hint},
		},
	}
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
