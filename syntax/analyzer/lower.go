package analyzer

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"znkr.io/markst/builtin"
	"znkr.io/markst/expr"
	"znkr.io/markst/internal/names"
	"znkr.io/markst/internal/symbols"
	"znkr.io/markst/name"
	"znkr.io/markst/syntax"
	"znkr.io/markst/syntax/convert"
	"znkr.io/markst/value"
)

// lowerMarkup lowers a markup node into a single SSA Ref. Labels are
// attached eagerly (via [expr.Builder.AttachLabel]) to the most recent
// preceding content item so subsequent ref expressions see them in the
// evaluator's label set.
func (a *analyzer) lowerMarkup(n syntax.Node) expr.Ref {
	items := a.lowerMarkupItems(n)
	switch len(items) {
	case 0:
		return a.b.Const(n.Span(), value.None{})
	case 1:
		return items[0]
	default:
		return a.b.ContentResult(n.Span(), items)
	}
}

// lowerMarkupItems walks a markup body, lowering each non-label child to a
// Ref (collected and returned) and emitting [AttachLabel] for each label
// child against the most recent preceding content item.
func (a *analyzer) lowerMarkupItems(n syntax.Node) []expr.Ref {
	var items []expr.Ref
	a.eachMarkupItem(n, func(ref expr.Ref, _ syntax.Span) {
		items = append(items, ref)
	})
	return items
}

// eachMarkupItem walks a markup body, invoking emit for each value-producing
// item in source order and attaching each label to the most recent preceding
// item via [AttachLabel] (detached labels are dropped). Shared by
// [lowerMarkupItems] and [lowerJoinedBlock] (content blocks). Once an escape
// becomes pending, the remaining items can never execute: they are not lowered
// (Typst stops evaluating the sequence at the same point), only their parse
// errors are adopted.
func (a *analyzer) eachMarkupItem(n syntax.Node, emit func(expr.Ref, syntax.Span)) {
	ns := a.inner(n, syntax.KindMarkup)
	last := expr.NoRef
	// swallow records that the preceding sibling absorbs the whitespace after
	// it: either it owns its line (see [ownsLine]) or it is a statement that
	// contributes nothing for a space to sit beside. False at the start of the
	// body, because a leading space there may still separate this run from
	// whatever it gets spliced into — `#emph[Hello ]world`.
	swallow := false
	entry := a.frame().pending
	for child := range ns.all() {
		if a.frame().pending != entry {
			a.adoptParseErrors(child)
			continue
		}
		switch child.Kind() {
		case syntax.KindError:
			last = a.emitSyntaxError(child.(*syntax.Error))
			emit(last, child.Span())
			swallow = false
		case syntax.KindLabel:
			if last == expr.NoRef {
				// Detached label: drop it. (Matches legacy "no preceding
				// content" behavior.)
				continue
			}
			label := a.leaf(child, syntax.KindLabel)
			labelName := name.Make(label[1 : len(label)-1])
			a.b.AttachLabel(child.Span(), last, labelName)
		case syntax.KindSpace:
			// A space with a block-level or statement neighbour has no two
			// words left to separate: `#let x = 1` swallows the line break
			// after it, exactly as a heading does.
			if swallow || a.ownsLine(ns.nextContent()) {
				continue
			}
			if ref := a.lowerExpr(child); ref != expr.NoRef {
				// A space is content, but it is never what a label names:
				// `text <label>` labels the text, not the space before the
				// label. So `last` is deliberately left alone here.
				emit(ref, child.Span())
			}
		default:
			ref := a.lowerExpr(child)
			if ref != expr.NoRef {
				last = ref
				emit(ref, child.Span())
			}
			swallow = ref == expr.NoRef || a.ownsLine(child)
		}
	}
}

// ownsLine reports whether n is markup that occupies a line of its own, so that
// whitespace beside it separates nothing.
//
// It is a static under-approximation of the question realization asks of the
// finished value — [value.Content.IsBlock], plus the two breaks. Every kind
// listed here realizes to content that answers yes no matter what, because even
// a show rule replacing one has its result realized in block context. Kinds
// whose blockness is only known at runtime are left out; answering no is always
// safe, since realization trims the space anyway.
//
// Being safe in that direction is what makes it an approximation rather than a
// rule: under [WithoutApproximations] it answers no to everything, every space
// is lowered, and realization produces the same document from a larger module.
func (a *analyzer) ownsLine(n syntax.Node) bool {
	if a.disableApprox || n == nil {
		return false
	}
	switch n.Kind() {
	case syntax.KindParbreak, syntax.KindLinebreak, syntax.KindHeading,
		syntax.KindListItem, syntax.KindEnumItem, syntax.KindTermItem:
		return true
	case syntax.KindEquation:
		return equationIsBlock(n)
	case syntax.KindRaw:
		return a.rawIsBlock(n)
	}
	return false
}

// rawIsBlock reports whether a raw node is delimited by more than one backtick,
// which is what makes it a block rather than an inline snippet. It mirrors the
// `block` decision in [analyzer.lowerRaw].
func (a *analyzer) rawIsBlock(n syntax.Node) bool {
	inner, ok := n.(*syntax.Inner)
	if !ok {
		return false
	}
	for _, child := range inner.Children() {
		if child.Kind() == syntax.KindRawDelim {
			return a.str(child) != "`"
		}
	}
	return false
}

// lowerExpr is the dispatch entry for converting a single code- or markup-
// mode syntax node into an SSA value reference. Statement-style nodes
// (let-bindings, set/show rules, assignments) return [expr.NoRef] to
// indicate they shouldn't contribute to a content sequence.
func (a *analyzer) lowerExpr(n syntax.Node) expr.Ref {
	switch n.Kind() {
	// Markup leaves
	case syntax.KindText:
		return a.b.Const(n.Span(), a.textValue(a.str(n)))
	case syntax.KindSpace:
		// A markup whitespace run is worth exactly one space: two blank lines
		// scan as a parbreak instead, so there is nothing longer to preserve.
		return a.b.Const(n.Span(), a.textValue(" "))
	case syntax.KindEscape:
		// In math, an escape like `\(` denotes a symbol character (matching
		// Typst, where it can stand in as a delimiter); in markup it is text.
		if a.mathDepth > 0 {
			return a.b.Const(n.Span(), &value.Symbol{Variants: symbols.Variants{{Value: unescape(a.str(n))}}})
		}
		return a.b.Const(n.Span(), a.textValue(unescape(a.str(n))))
	case syntax.KindShorthand:
		return a.b.Const(n.Span(), a.textValue(unshorthand(a.str(n))))
	case syntax.KindSmartQuote:
		return a.b.Const(n.Span(), &value.SmartQuote{Double: a.str(n) == `"`})
	case syntax.KindLinebreak:
		return a.b.Const(n.Span(), &value.Linebreak{})
	case syntax.KindParbreak:
		return a.b.Const(n.Span(), &value.Parbreak{})
	case syntax.KindLabel:
		// Labels appearing in expression position (e.g. as a function
		// argument) materialize as a Const Label value. Labels appearing
		// inside markup are intercepted by lowerMarkupItems and emit an
		// AttachLabel instruction instead.
		label := a.leaf(n, syntax.KindLabel)
		label = label[1 : len(label)-1]
		return a.b.Const(n.Span(), &value.Label{Name: name.Make(label)})
	case syntax.KindRaw:
		return a.lowerRaw(n)
	// Markup constructs
	case syntax.KindHeading:
		return a.lowerHeading(n)
	case syntax.KindStrong:
		return a.lowerStrong(n)
	case syntax.KindEmph:
		return a.lowerEmph(n)
	case syntax.KindLink:
		lit := a.str(n)
		body := a.b.Const(n.Span(), a.textValue(lit))
		return a.b.Link(n.Span(), lit, body)
	case syntax.KindRef:
		return a.lowerRef(n)
	case syntax.KindListItem:
		return a.lowerListItem(n)
	case syntax.KindEnumItem:
		return a.lowerEnumItem(n)
	case syntax.KindTermItem:
		return a.lowerTermItem(n)
	case syntax.KindContentBlock:
		return a.lowerContentBlock(n)
	// Code literals
	case syntax.KindBool:
		return a.lowerBool(n)
	case syntax.KindInt:
		return a.lowerInt(n)
	case syntax.KindFloat:
		return a.lowerFloat(n)
	case syntax.KindNumeric:
		return a.lowerNumeric(n)
	case syntax.KindStr:
		return a.lowerStr(n)
	case syntax.KindNone:
		return a.b.Const(n.Span(), value.None{})
	case syntax.KindAuto:
		return a.b.Const(n.Span(), value.Auto{})
	case syntax.KindIdent:
		return a.lowerIdent(n)
	case syntax.KindParenthesized:
		return a.lowerParenthesized(n)
	case syntax.KindUnary:
		return a.lowerUnary(n)
	case syntax.KindBinary:
		return a.lowerBinary(n)
	case syntax.KindArray:
		return a.lowerArray(n)
	case syntax.KindDict:
		return a.lowerDict(n)
	case syntax.KindFieldAccess:
		return a.lowerFieldAccess(n)
	case syntax.KindFuncCall:
		return a.lowerFuncCall(n)
	case syntax.KindLetBinding:
		return a.lowerLetBinding(n)
	case syntax.KindCodeBlock:
		return a.lowerCodeBlock(n)
	case syntax.KindConditional:
		return a.lowerConditional(n)
	case syntax.KindWhileLoop:
		return a.lowerWhileLoop(n)
	case syntax.KindForLoop:
		return a.lowerForLoop(n)
	case syntax.KindLoopBreak:
		return a.lowerLoopBreak(n)
	case syntax.KindLoopContinue:
		return a.lowerLoopContinue(n)
	case syntax.KindFuncReturn:
		return a.lowerFuncReturn(n)
	case syntax.KindClosure:
		return a.lowerClosure(n)
	case syntax.KindDestructAssignment:
		return a.lowerDestructAssignment(n)
	case syntax.KindSetRule:
		return a.lowerSetRule(n)
	case syntax.KindShowRule:
		return a.lowerShowRule(n)
	case syntax.KindContextual:
		return a.lowerContextual(n)
	case syntax.KindModuleInclude:
		return a.lowerModuleInclude(n)
	case syntax.KindModuleImport:
		// The parser understands the full import grammar, but nothing below it
		// does. Report it rather than falling through to the panic below: this
		// is reachable from ordinary source, and the shape a host reaches for
		// first when told that markst has libraries.
		return a.emitError(n.Span(), "imports are not supported",
			"a library's bindings are supplied by the host and need no import; see markst.CompileLibrary")
	case syntax.KindError:
		return a.emitSyntaxError(n.(*syntax.Error))
	// Math
	case syntax.KindEquation:
		return a.lowerEquation(n)
	case syntax.KindMath:
		return a.lowerMathContent(n)
	case syntax.KindMathText:
		return a.lowerMathText(n)
	case syntax.KindMathIdent:
		return a.lowerMathIdent(n)
	case syntax.KindMathShorthand:
		return a.b.Const(n.Span(), a.mathTextValue(mathShorthand(a.str(n))))
	case syntax.KindMathAlignPoint:
		return a.b.Const(n.Span(), &value.MathAlignPoint{})
	case syntax.KindMathAttach:
		return a.lowerMathAttach(n)
	case syntax.KindMathFrac:
		return a.lowerMathFrac(n)
	case syntax.KindMathRoot:
		return a.lowerMathRoot(n)
	case syntax.KindMathPrimes:
		return a.lowerMathPrimes(n)
	case syntax.KindMathDelimited:
		return a.lowerMathDelimited(n)
	case syntax.KindMathCall:
		return a.lowerMathCall(n)
	default:
		panic("ssa lowering not yet implemented for: " + n.Kind().String())
	}
}

// Literals ////////////////////////////////////////////////////////////////////

func (a *analyzer) lowerBool(n syntax.Node) expr.Ref {
	val := a.leaf(n, syntax.KindBool)
	v, ok := bools[val]
	if !ok {
		panic("invalid bool literal: " + val)
	}
	return a.b.Const(n.Span(), v)
}

func (a *analyzer) lowerInt(n syntax.Node) expr.Ref {
	val := a.leaf(n, syntax.KindInt)
	iv, err := convert.ParseInt(val)
	if err != nil {
		panic(err.Error())
	}
	return a.b.Const(n.Span(), value.Int(iv))
}

func (a *analyzer) lowerFloat(n syntax.Node) expr.Ref {
	val := a.leaf(n, syntax.KindFloat)
	fv, err := strconv.ParseFloat(val, 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		panic("invalid float literal: " + val)
	}
	return a.b.Const(n.Span(), value.Float(fv))
}

// isUnitByte reports whether c can appear in the unit suffix of a numeric
// literal (`pt`, `deg`, `%`, …). Every suffix the scanner accepts is ASCII.
func isUnitByte(c byte) bool {
	return c == '%' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
}

func (a *analyzer) lowerNumeric(n syntax.Node) expr.Ref {
	val := a.leaf(n, syntax.KindNumeric)
	// Split off the unit the way the scanner did: it is the trailing run of
	// ASCII letters, or `%`. Scanning from the front instead would cut a
	// scientific-notation exponent in half — `0e0cm` has the unit `cm`, not
	// `e0cm`.
	idx := len(val)
	for idx > 0 && isUnitByte(val[idx-1]) {
		idx--
	}
	if idx == len(val) {
		panic("invalid numeric literal, no unit: " + val)
	}
	num, suffix := val[:idx], val[idx:]
	fv, err := strconv.ParseFloat(num, 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		panic("invalid float literal: " + val)
	}
	var v value.Value
	switch suffix {
	case "pt":
		v = value.Length{Pt: fv}
	case "mm":
		v = value.Length{Pt: fv * (72.0 / 25.4)}
	case "cm":
		v = value.Length{Pt: fv * (72.0 / 2.54)}
	case "in":
		v = value.Length{Pt: fv * 72.0}
	case "em":
		v = value.Length{Em: fv}
	case "deg":
		v = value.Angle(fv / 180.0 * math.Pi)
	case "rad":
		v = value.Angle(fv)
	case "fr":
		v = value.Fraction(fv)
	case "%":
		v = value.Ratio(fv / 100.0)
	default:
		panic("invalid unit literal: " + val)
	}
	return a.b.Const(n.Span(), v)
}

func (a *analyzer) lowerStr(n syntax.Node) expr.Ref {
	val := a.leaf(n, syntax.KindStr)
	return a.b.Const(n.Span(), value.Str(unquote(val)))
}

// Identifiers /////////////////////////////////////////////////////////////////

func (a *analyzer) lowerIdent(n syntax.Node) expr.Ref {
	source := name.Make(a.leaf(n, syntax.KindIdent))
	if errRef := a.checkIdent(source, n.Span()); errRef != expr.NoRef {
		return errRef
	}
	return a.resolveName(source, n.Span())
}

// resolveName produces the SSA Ref in the current function for source.
// Walks the scope chain: builtin/user-binding → inline [expr.Const] with the
// resolved value; self-binding (recursion name) at depth 0 → [expr.Builder.Self]
// in the current builder; local (no boundary crossed) → ReadVar with the
// SSA variable; outer (one or more boundaries crossed) → allocate a
// capture in the innermost active closure. Transitive captures (the binding
// sits outside more than one closure) work because lowerClosure resolves
// each capture's outer Ref in its enclosing builder via resolveName, which
// recurses through any intermediate closures.
func (a *analyzer) resolveName(source name.Name, span syntax.Span) expr.Ref {
	return a.resolveNameIn(source, span, false)
}

// resolveMathName is [analyzer.resolveName] for a math identifier, which sees
// the same chain [analyzer.lookupMath] does: everything written around the
// equation and the math module, but not the universe.
func (a *analyzer) resolveMathName(source name.Name, span syntax.Span) expr.Ref {
	return a.resolveNameIn(source, span, true)
}

func (a *analyzer) resolveNameIn(source name.Name, span syntax.Span, mathOnly bool) expr.Ref {
	frameIdx := len(a.frames) - 1
	inCurrent := true
	var fallback value.ModuleDef
	for s := a.scope; s != nil; s = s.parent {
		if mathOnly && s.parent == nil {
			break // the universe: reachable from code, not from math
		}
		if s.mathScope != nil {
			fallback = s.mathScope
		}
		if bnd, ok := s.bindings[source]; ok {
			switch b := bnd.(type) {
			case valueBinding:
				return a.b.Const(span, b.val)
			case selfBinding:
				if inCurrent {
					return a.b.Self(span)
				}
				return a.captureRef(source, span)
			case varBinding:
				if inCurrent {
					return a.b.ReadVar(b.v, a.b.CurrentBlock())
				}
				// Found the name outside the current closure.
				f := a.frames[frameIdx]
				if ref, ok := f.b.PeekVar(b.v); ok && ref.IsModConst() {
					// Module constants are immutable, no capture needed.
					return ref
				}
				return a.captureRef(source, span)
			default:
				panic(fmt.Sprintf("unknown binding kind: %T", b))
			}
		}
		if frameIdx >= 0 && s == a.frames[frameIdx].scope {
			inCurrent = false
			frameIdx--
		}
	}
	// A math-scope fallback (installed on the equation body scope) resolves
	// symbols and math elements to immutable constants.
	if fallback != nil {
		if v := fallback.Get(source); v != nil {
			return a.b.Const(span, v)
		}
		// `std` is the way into the universe from math (see analyzer.lookupMath).
		if source == names.Std {
			if v := builtin.Universe[names.Std]; v != nil {
				return a.b.Const(span, v)
			}
		}
	}
	// checkIdent already reported "unknown variable" — this is a bug if we
	// get here, but we don't want to crash silently.
	return expr.NoRef
}

// captureRef returns the capture Ref for source in the current
// frame, allocating one on first reference. Captures are never reassignable,
// so no SSA-variable indirection is needed — the returned Ref is the
// canonical value for source in this frame.
func (a *analyzer) captureRef(source name.Name, span syntax.Span) expr.Ref {
	f := a.frame()
	if ref, ok := f.captures[source]; ok {
		return ref
	}
	ref := a.b.AddCapture(source, span)
	if f.captures == nil {
		f.captures = make(map[name.Name]expr.Ref)
	}
	f.captures[source] = ref
	return ref
}

// Operators ///////////////////////////////////////////////////////////////////

func (a *analyzer) lowerUnary(n syntax.Node) expr.Ref {
	ns := a.inner(n, syntax.KindUnary)
	op := syntax.UnaryOpFromKind(ns.node().Kind())
	x := a.lowerExpr(ns.node())
	return a.b.Unary(n.Span(), op, x)
}

func (a *analyzer) lowerBinary(n syntax.Node) expr.Ref {
	ns := a.inner(n, syntax.KindBinary)
	leftNode := ns.node()
	var op syntax.BinaryOp
	if ns.at(syntax.KindNot) {
		ns.take(syntax.KindNot)
		ns.take(syntax.KindIn)
		op = syntax.NotIn
	} else {
		op = syntax.BinaryOpFromKind(ns.node().Kind())
	}
	rightNode := ns.node()
	if op.IsAssign() {
		return a.lowerAssign(n.Span(), op, leftNode, rightNode)
	}
	if op == syntax.And || op == syntax.Or {
		return a.lowerShortCircuit(n.Span(), op, leftNode, rightNode)
	}
	left := a.lowerExpr(leftNode)
	right := a.lowerExpr(rightNode)
	return a.b.Binary(n.Span(), op, left, right)
}

// lowerShortCircuit lowers `left and right` / `left or right` into a CFG that
// evaluates the right operand only when needed. The left operand is always
// evaluated (and required to be a boolean by the [expr.Branch] terminator); the
// right operand is only evaluated when the result isn't already determined by
// the left.
func (a *analyzer) lowerShortCircuit(span syntax.Span, op syntax.BinaryOp, leftNode, rightNode syntax.Node) expr.Ref {
	left := a.lowerExpr(leftNode)
	result := a.b.NewVar(name.Make("$sc"))
	a.b.WriteVar(result, a.b.CurrentBlock(), left)

	rightBlk := a.b.NewBlock()
	join := a.b.NewBlock()
	if op == syntax.And {
		// false short-circuits: skip right; result already = left (false).
		a.b.Branch(span, left, rightBlk, join)
	} else {
		// true short-circuits: skip right; result already = left (true).
		a.b.Branch(span, left, join, rightBlk)
	}

	a.b.SetBlock(rightBlk)
	a.b.SealBlock(rightBlk)
	right := a.lowerExpr(rightNode)
	combined := a.b.Binary(span, op, left, right)
	a.b.WriteVar(result, a.b.CurrentBlock(), combined)
	a.b.Jump(span, join)

	a.b.SetBlock(join)
	a.b.SealBlock(join)
	return a.b.ReadVar(result, join)
}

// isCapturedVar reports whether source resolves as a capture in the current
// function, i.e. it is defined outside the innermost closure boundary.
func (a *analyzer) isCapturedVar(source name.Name) bool {
	if len(a.frames) == 1 {
		// No closures, so no captures.
		return false
	}
	boundary := a.frame().scope
	inCurrent := true
	for s := a.scope; s != nil; s = s.parent {
		if _, ok := s.bindings[source]; ok {
			return !inCurrent
		}
		if s == boundary {
			inCurrent = false
		}
	}
	return false
}

// lvalueBase returns the leftmost identifier node in a FieldAccess / FuncCall
// lvalue chain (recursing through callee and target), or (nil, false) when the
// base is not a plain identifier.
func lvalueBase(n syntax.Node) (syntax.Node, bool) {
	for {
		switch n.Kind() {
		case syntax.KindIdent:
			return n, true
		case syntax.KindFieldAccess, syntax.KindFuncCall:
			inner, ok := n.(*syntax.Inner)
			if !ok {
				return nil, false
			}
			// Walk to the first non-trivia child (target/callee).
			var next syntax.Node
			for _, ch := range inner.Children() {
				k := ch.Kind()
				if k != syntax.KindSpace && k != syntax.KindLineComment && k != syntax.KindBlockComment {
					next = ch
					break
				}
			}
			if next == nil {
				return nil, false
			}
			n = next
		default:
			return nil, false
		}
	}
}

// lowerAssign handles `=`, `+=`, `-=`, `*=`, `/=`. The LHS must currently be
// an ident; FieldAccess and call-form LHSes are deferred to a later batch.
func (a *analyzer) lowerAssign(span syntax.Span, op syntax.BinaryOp, leftNode, rightNode syntax.Node) expr.Ref {
	switch leftNode.Kind() {
	case syntax.KindIdent:
		if op == syntax.Assign {
			newVal := a.lowerExpr(rightNode)
			return a.writeLValue(span, leftNode, syntax.Assign, newVal)
		}
		// Compound assignment to an ident needs the old value, so it can't go
		// through writeLValue. Match Typst semantics: RHS is computed first
		// (may shadow the LHS via side-effects), then the LHS's current value
		// is read, then combined with the RHS.
		//
		// This feels more like an accident than a deliberate design choice,
		// but here we are. For reference, here is an example that shadows
		// the LHS via a let binding on the RHS:
		//
		//   #{
		//     let var = "a"
		//     var += var.at(0, default: let var = "b")
		//     test(var, "ba")
		//   }
		source := name.Make(a.leaf(leftNode, syntax.KindIdent))
		bnd, ok := a.lookup(source)
		if !ok {
			return a.checkIdent(source, leftNode.Span())
		}
		rhs := a.lowerExpr(rightNode)
		// Re-resolve in case the RHS shadowed the binding via a `let`.
		if b, ok := a.lookup(source); ok {
			bnd = b
		}
		v, errRef, ok := a.assignVar(bnd, source, leftNode.Span())
		if !ok {
			return errRef
		}
		old := a.b.ReadVar(v, a.b.CurrentBlock())
		newVal := a.b.Binary(span, op.StripAssign(), old, rhs)
		a.b.WriteVar(v, a.b.CurrentBlock(), newVal)
		return a.b.Const(span, value.None{})
	case syntax.KindParenthesized:
		// Unwrap and recurse.
		ns := a.inner(leftNode, syntax.KindParenthesized)
		var inner syntax.Node
		for child := range ns.inside(syntax.KindLeftParen, syntax.KindRightParen) {
			inner = child
		}
		return a.lowerAssign(span, op, inner, rightNode)
	case syntax.KindFieldAccess, syntax.KindFuncCall:
		// `x.f = v` / `f(args) = v` carry the (stripped) op into FieldWrite /
		// CallSet, which perform the read-modify-write for compound ops.
		stripped := op
		if op != syntax.Assign {
			stripped = op.StripAssign()
		}
		newVal := a.lowerExpr(rightNode)
		return a.writeLValue(span, leftNode, stripped, newVal)
	default:
		// Genuinely-temporary lvalues (e.g. `(1+2) = 3`). writeLValue lowers
		// the LHS for its side effects and emits the deferred "cannot mutate"
		// error; the RHS is not evaluated.
		return a.writeLValue(span, leftNode, op, expr.NoRef)
	}
}

// writeLValue stores newVal into the lvalue leftNode, returning the value of
// the assignment expression (none on success, or an error ref). op is the
// already-stripped binary op (Assign for a plain store); for field/call
// lvalues it carries the read-modify-write into FieldWrite/CallSet. This is
// shared by lowerAssign and destructuring-assignment leaves (see pattern.go),
// which pass an already-lowered element as newVal. leftNode is never an ident
// in the destructuring path (those go through bindLeaf).
func (a *analyzer) writeLValue(span syntax.Span, leftNode syntax.Node, op syntax.BinaryOp, newVal expr.Ref) expr.Ref {
	switch leftNode.Kind() {
	case syntax.KindIdent:
		source := name.Make(a.leaf(leftNode, syntax.KindIdent))
		bnd, ok := a.lookup(source)
		if !ok {
			return a.checkIdent(source, leftNode.Span())
		}
		v, errRef, ok := a.assignVar(bnd, source, leftNode.Span())
		if !ok {
			return errRef
		}
		a.b.WriteVar(v, a.b.CurrentBlock(), newVal)
		return a.b.Const(span, value.None{})
	case syntax.KindFieldAccess:
		if baseNode, ok := lvalueBase(leftNode); ok {
			source := name.Make(a.str(baseNode))
			if a.isCapturedVar(source) {
				return a.emitError(baseNode.Span(), "variables from outside the function are read-only and cannot be modified")
			}
		}
		fns := a.inner(leftNode, syntax.KindFieldAccess)
		targetNode := fns.node()
		target := a.lowerExpr(targetNode)
		fns.take(syntax.KindDot)
		fieldName := name.Make(a.leaf(fns.node(), syntax.KindIdent))
		// Use the LHS span for the FieldWrite so runtime errors point at the
		// field-access expression, not the whole assignment.
		a.b.FieldWrite(targetNode.Span(), target, fieldName, newVal, op)
		return a.b.Const(span, value.None{})
	case syntax.KindFuncCall:
		if baseNode, ok := lvalueBase(leftNode); ok {
			source := name.Make(a.str(baseNode))
			if a.isCapturedVar(source) {
				return a.emitError(baseNode.Span(), "variables from outside the function are read-only and cannot be modified")
			}
		}
		fns := a.inner(leftNode, syntax.KindFuncCall)
		calleeNode := fns.node()
		callee := expr.Callee{Ref: a.lowerCallee(calleeNode), Span: calleeNode.Span()}
		args, blocks := a.lowerArgs(fns.node())
		// Use the LHS span so runtime errors point at the call expression.
		a.b.CallSet(leftNode.Span(), callee, args, blocks, newVal, op)
		return a.b.Const(span, value.None{})
	case syntax.KindBinary, syntax.KindUnary:
		// Lower the LHS so any runtime errors fire first (matching legacy where
		// the eval error of the LHS supersedes "cannot mutate"); then emit a
		// deferred Error wired to the LHS so the diagnostic only fires when the
		// LHS itself didn't already error.
		from := a.lowerExpr(leftNode)
		a.b.Error(leftNode.Span(), "cannot mutate a temporary value", from)
		return expr.NoRef
	default:
		a.b.Error(leftNode.Span(), "cannot mutate a temporary value", expr.NoRef)
		return expr.NoRef
	}
}

// Grouping ////////////////////////////////////////////////////////////////////

func (a *analyzer) lowerParenthesized(n syntax.Node) expr.Ref {
	ns := a.inner(n, syntax.KindParenthesized)
	ref := expr.NoRef
	for child := range ns.inside(syntax.KindLeftParen, syntax.KindRightParen) {
		ref = a.lowerExpr(child)
	}
	return ref
}

// Collections /////////////////////////////////////////////////////////////////

func (a *analyzer) lowerArray(n syntax.Node) expr.Ref {
	ns := a.inner(n, syntax.KindArray)
	var items []expr.ArrayItem
	for child := range ns.inside(syntax.KindLeftParen, syntax.KindRightParen) {
		switch kind := child.Kind(); kind {
		case syntax.KindComma:
			continue
		case syntax.KindSpread:
			items = append(items, expr.ArrayItem{Value: a.lowerSpread(child), Spread: true, Span: child.Span()})
		case syntax.KindNamed, syntax.KindKeyed:
			a.emitError(child.Span(), "expected expression, found "+kind.Name()+" pair")
		default:
			ref := a.lowerExpr(child)
			if ref == expr.NoRef {
				continue
			}
			items = append(items, expr.ArrayItem{Value: ref, Span: child.Span()})
		}
	}
	return a.b.MakeArray(n.Span(), items)
}

func (a *analyzer) lowerSpread(n syntax.Node) expr.Ref {
	ns := a.inner(n, syntax.KindSpread)
	ns.take(syntax.KindDots)
	x := a.lowerExpr(ns.node())

	return x
}

func (a *analyzer) lowerDict(n syntax.Node) expr.Ref {
	ns := a.inner(n, syntax.KindDict)
	var entries []expr.DictEntry
	seen := make(map[string]struct{})
	for child := range ns.inside(syntax.KindLeftParen, syntax.KindRightParen) {
		switch child.Kind() {
		case syntax.KindComma, syntax.KindColon:
			continue
		case syntax.KindNamed:
			entry := a.inner(child, syntax.KindNamed)
			key, ok := entry.take(syntax.KindIdent)
			if !ok {
				continue
			}
			entry.take(syntax.KindColon)
			if _, dup := seen[key]; dup {
				a.emitError(child.Span(), "duplicate key: "+key)
			}
			seen[key] = struct{}{}
			keyRef := a.b.Const(child.Span(), value.Str(key))
			val := a.lowerExpr(entry.node())

			if val == expr.NoRef {
				continue
			}
			entries = append(entries, expr.DictEntry{Key: keyRef, Value: val})
		case syntax.KindKeyed:
			entry := a.inner(child, syntax.KindKeyed)
			var keyStr string
			if entry.at(syntax.KindStr) {
				if leaf, ok := a.peekLeafText(&entry); ok {
					keyStr = unquote(leaf)
				}
			}
			keyRef := a.lowerExpr(entry.node())
			entry.take(syntax.KindColon)
			val := a.lowerExpr(entry.node())

			if keyRef == expr.NoRef || val == expr.NoRef {
				continue
			}
			if keyStr != "" {
				if _, dup := seen[keyStr]; dup {
					a.emitError(child.Span(), "duplicate key: "+keyStr)
				}
				seen[keyStr] = struct{}{}
			}
			entries = append(entries, expr.DictEntry{Key: keyRef, Value: val})
		case syntax.KindSpread:
			entries = append(entries, expr.DictEntry{Key: expr.NoRef, Value: a.lowerSpread(child), Spread: true})
		case syntax.KindError:
			a.emitSyntaxError(child.(*syntax.Error))
		default:
			a.emitError(child.Span(), "expected named or keyed pair")
		}
	}
	return a.b.MakeDict(n.Span(), entries)
}

// peekLeafText returns the text of the current node if it is a *syntax.Leaf.
// Used to peek at string-literal keys for compile-time duplicate detection
// without consuming the node.
func (a *analyzer) peekLeafText(ns *nodes) (string, bool) {
	if ns.pos >= len(ns.items) {
		return "", false
	}
	if leaf, ok := ns.items[ns.pos].(*syntax.Leaf); ok {
		return a.str(leaf), true
	}
	return "", false
}

// Field access ////////////////////////////////////////////////////////////////

func (a *analyzer) lowerFieldAccess(n syntax.Node) expr.Ref {
	ns := a.inner(n, syntax.KindFieldAccess)
	target := a.lowerExpr(ns.node())
	ns.node() // consume `.`
	fieldNode := ns.node()
	fieldName := name.Make(a.leaf(fieldNode, syntax.KindIdent))
	return a.b.FieldRead(n.Span(), fieldNode.Span(), target, fieldName)
}

// lowerCallee lowers the callee of a call to its value Ref. A `target.method`
// callee resolves with method-call semantics ([Builder.MethodField]) so a
// missing member reports as a missing method rather than a missing field;
// anything else is lowered as an ordinary value. The method-vs-field choice is
// positional — it belongs to the caller that knows the field access sits in
// callee position, not to [lowerExpr], which lowers a field access to a plain
// field read in every value context.
func (a *analyzer) lowerCallee(calleeNode syntax.Node) expr.Ref {
	if calleeNode.Kind() != syntax.KindFieldAccess {
		return a.lowerExpr(calleeNode)
	}
	ns := a.inner(calleeNode, syntax.KindFieldAccess)
	targetNode := ns.node()
	ns.take(syntax.KindDot)
	fieldNode := ns.node()
	method := name.Make(a.leaf(fieldNode, syntax.KindIdent))
	return a.b.MethodField(calleeNode.Span(), fieldNode.Span(), a.lowerExpr(targetNode), method, a.str(targetNode), false)
}

// Function calls //////////////////////////////////////////////////////////////

func (a *analyzer) lowerFuncCall(n syntax.Node) expr.Ref {
	ns := a.inner(n, syntax.KindFuncCall)
	calleeNode := ns.node()
	argsNode := ns.node()
	// `target.method(args)` is a method call, which goes through MethodField
	// dispatch and a receiver place check distinct from a plain
	// field-access-then-call.
	if calleeNode.Kind() == syntax.KindFieldAccess {
		ref, _ := a.lowerMethodCall(n, calleeNode, argsNode)
		return ref
	}
	callee := expr.Callee{Ref: a.lowerExpr(calleeNode), Span: calleeNode.Span()}
	args, blocks := a.lowerArgs(argsNode)
	return a.b.Call(n.Span(), callee, args, blocks, false, nil)
}

// placeInfo records whether a method-call receiver denotes a mutable place. The
// receiver is a place iff its chain roots in a variable (temporary is false)
// and every method link resolves to an accessor; accessors holds those links'
// callee Refs, checked at runtime via [value.Function.Accessor]. temporary true
// means the receiver is definitely a temporary — a literal/arbitrary-expression
// root, or a plain (non-method) call somewhere in the chain — and accessors is
// then irrelevant.
type placeInfo struct {
	temporary bool
	accessors []expr.Ref
}

// lowerReceiver lowers the receiver of a method call, returning its value Ref
// plus the place info an enclosing mutating call needs. It mirrors lowerExpr for
// the shapes that can form a mutable place — identifiers, field accesses, and
// method-call links — and falls back to lowerExpr (marking the result a
// temporary) for everything else.
func (a *analyzer) lowerReceiver(n syntax.Node) (expr.Ref, placeInfo) {
	switch n.Kind() {
	case syntax.KindIdent:
		// A bare variable is an unconditional place.
		return a.lowerExpr(n), placeInfo{}
	case syntax.KindFieldAccess:
		// A field-access link preserves the base's place-ness.
		ns := a.inner(n, syntax.KindFieldAccess)
		baseRef, base := a.lowerReceiver(ns.node())
		ns.node() // consume `.`
		fieldNode := ns.node()
		fieldName := name.Make(a.leaf(fieldNode, syntax.KindIdent))
		return a.b.FieldRead(n.Span(), fieldNode.Span(), baseRef, fieldName), base
	case syntax.KindFuncCall:
		ns := a.inner(n, syntax.KindFuncCall)
		calleeNode := ns.node()
		argsNode := ns.node()
		if calleeNode.Kind() == syntax.KindFieldAccess {
			return a.lowerMethodCall(n, calleeNode, argsNode)
		}
		// A plain function call yields a fresh value, never a place.
		return a.lowerExpr(n), placeInfo{temporary: true}
	default:
		return a.lowerExpr(n), placeInfo{temporary: true}
	}
}

// lowerMethodCall lowers `target.method(args)`, returning the result Ref and the
// place info of the whole expression (for use when this call is itself the
// receiver of an outer mutating method call). Dispatch goes through a
// [Builder.MethodField] callee so the runtime applies method-call error
// semantics (dictionary keys are not directly callable; a missing field on a
// content element or method-bearing type is reported as a missing method).
//
// Evaluation order is uniform: the receiver is evaluated first, then the
// arguments left-to-right — the same order as a plain function call and every
// other expression in the language. Unlike Typst, mutating methods do not get a
// separate arguments-first order; the two orders differ only when an argument's
// side effects alias the receiver's place (e.g.
// `arr.at(pair.remove(0)).push(pair.remove(0))`), which is exactly the case a
// runtime aliasing guard would flag. See "Method-call evaluation order: aliasing
// guard" in IDEAS.md.
//
// A mutating method requires the receiver to be a mutable place; a mutating call
// on a temporary reports "cannot mutate a temporary value". Both mutating-ness
// (from the resolved method's [value.Function.Impure]) and place-ness (from the
// resolved links' [value.Function.Accessor]) are decided at runtime — the
// receiver's type isn't known here — so the emitted [expr.MutCheck] carries only
// the static shape of the receiver chain.
func (a *analyzer) lowerMethodCall(callNode, faNode, argsNode syntax.Node) (expr.Ref, placeInfo) {
	fns := a.inner(faNode, syntax.KindFieldAccess)
	targetNode := fns.node()
	fns.take(syntax.KindDot)
	fieldNode := fns.node()
	method := name.Make(a.leaf(fieldNode, syntax.KindIdent))

	targetRef, recv := a.lowerReceiver(targetNode)
	calleeRef := a.b.MethodField(faNode.Span(), fieldNode.Span(), targetRef, method, a.str(targetNode), false)
	callee := expr.Callee{Ref: calleeRef, Span: faNode.Span()}
	args, blocks := a.lowerArgs(argsNode)

	mut := &expr.MutCheck{
		RecvSpan:      targetNode.Span(),
		RecvTemporary: recv.temporary,
		RecvAccessors: recv.accessors,
	}
	ref := a.b.Call(callNode.Span(), callee, args, blocks, false, mut)

	// `target.method(args)` is itself a place chain-link for an enclosing
	// mutating call: a place iff the receiver was a place and this method
	// resolves to an accessor (calleeRef). A fresh accessors slice avoids
	// aliasing the one handed to mut above.
	if recv.temporary {
		return ref, placeInfo{temporary: true}
	}
	return ref, placeInfo{accessors: append(append([]expr.Ref(nil), recv.accessors...), calleeRef)}
}

func (a *analyzer) lowerArgs(n syntax.Node) ([]expr.CallArg, []expr.Ref) {
	ns := a.inner(n, syntax.KindArgs)

	var args []expr.CallArg
	seen := make(map[string]struct{})
	if ns.at(syntax.KindLeftParen) || ns.at(syntax.KindError) {
		for child := range ns.inside(syntax.KindLeftParen, syntax.KindRightParen) {
			switch child.Kind() {
			case syntax.KindComma:
				continue
			case syntax.KindSpread:
				args = append(args, expr.CallArg{Kind: expr.ArgSpread, Value: a.lowerSpread(child), Span: child.Span()})
			case syntax.KindNamed:
				named := a.inner(child, syntax.KindNamed)
				keyNode := named.node()
				keyText := a.leaf(keyNode, syntax.KindIdent)
				key := name.Make(keyText)
				named.take(syntax.KindColon)
				valNode := named.node()
				val := a.lowerExpr(valNode)

				if _, dup := seen[key.String()]; dup {
					a.emitError(keyNode.Span(), "duplicate argument: "+key.String())
				}
				seen[key.String()] = struct{}{}
				args = append(args, expr.CallArg{Kind: expr.ArgNamed, Name: key, Value: val, Span: valNode.Span(), PairSpan: child.Span()})
			default:
				val := a.lowerExpr(child)
				ca := expr.CallArg{Kind: expr.ArgPositional, Value: val, Span: child.Span()}
				if child.Kind() == syntax.KindFloat {
					ca.DirectFloatLit = true
				}
				args = append(args, ca)
			}
		}
	}

	var blocks []expr.Ref
	for ns.at(syntax.KindContentBlock) {
		blocks = append(blocks, a.lowerContentBlock(ns.node()))
	}
	return args, blocks
}

// Let bindings ////////////////////////////////////////////////////////////////

func (a *analyzer) lowerLetBinding(n syntax.Node) expr.Ref {
	ns := a.inner(n, syntax.KindLetBinding)
	ns.take(syntax.KindLet)
	if ns.at(syntax.KindClosure) {
		closureNode := ns.node()
		// Extract the closure's name (function-shorthand `let f(x) = body`).
		recName, ok := a.closureSourceName(closureNode)
		if !ok {
			// Emit the legacy "expected identifier or parameters" error at
			// the would-be-name node's span. The closure node's first child
			// is the offending kind (e.g. a parenthesised pattern).
			inner, _ := closureNode.(*syntax.Inner)
			if inner != nil && len(inner.Children()) > 0 {
				first := inner.Children()[0]
				a.emitError(first.Span(), "expected identifier or parameters")
			} else {
				a.emitError(closureNode.Span(), "expected identifier or parameters")
			}
			return expr.NoRef
		}
		rhs := a.lowerClosureNamed(closureNode, recName)
		v := a.allocVar(recName)
		a.b.WriteVar(v, a.b.CurrentBlock(), rhs)
		return expr.NoRef
	}
	patternNode := ns.node()
	// Bail if the parser substituted an error for the pattern. The error
	// is already on the list; continuing would emit a destructure-against-
	// None instruction that produces a misleading "cannot destructure" at
	// runtime.
	if patternNode.Kind() == syntax.KindError {
		a.emitSyntaxError(patternNode.(*syntax.Error))
		// Don't try to lower the RHS — it may reference the pattern's bindings,
		// but with no pattern there's nothing meaningful to do with the value.
		return expr.NoRef
	}
	// Note: `let f = (..) => body` (an anonymous closure bound to a name) is
	// deliberately *not* self-referential — unlike the `let f(..) = body`
	// function-shorthand form. Inside the body, `f` resolves to the outer
	// binding (or is unknown), because the new binding is not yet in scope
	// within its own initializer. It falls through to the normal destructuring
	// path below and is lowered as a plain anonymous closure.
	var rhs expr.Ref
	if ns.at(syntax.KindEq) {
		ns.node() // consume eq
		rhs = a.lowerExpr(ns.node())
	} else if ns.at(syntax.KindError) {
		a.emitSyntaxError(ns.node().(*syntax.Error))
		return expr.NoRef
	} else {
		// `let x` without initializer binds to none.
		rhs = a.b.Const(n.Span(), value.None{})
	}

	a.lowerDestructPattern(patternNode, rhs)
	return expr.NoRef
}

// lowerDestructPattern binds names from a destructuring pattern to portions of
// the RHS. Recurses into nested patterns and handles both array-shape
// (positional) and dict-shape (named) destructuring with optional sinks.
func (a *analyzer) lowerDestructPattern(n syntax.Node, rhs expr.Ref) {
	pc := &patternCtx{a: a, seen: make(map[name.Name]struct{})}
	pc.compile(n, rhs)
}

// assignDestructPattern is the assignment counterpart of
// [analyzer.lowerDestructPattern]: each pattern leaf is an existing binding (or
// an arbitrary lvalue expression) that the runtime should overwrite, not a
// fresh binding to allocate.
func (a *analyzer) assignDestructPattern(n syntax.Node, rhs expr.Ref) {
	pc := &patternCtx{a: a, assign: true}
	pc.compile(n, rhs)
}

// Code blocks ////////////////////////////////////////////////////////////////

// lowerCodeBlock opens a new scope and lowers a code block's statements inline
// into the current basic block (code blocks introduce no control flow on their
// own), joining their values with the code-mode joiner. Each statement emits its
// value-producing item in source order; statement-style nodes that yield
// [expr.NoRef] (let-bindings, assignments, set/show rules) are skipped.
func (a *analyzer) lowerCodeBlock(n syntax.Node) expr.Ref {
	a.openScope()
	defer a.closeScope()
	return a.lowerJoinedBlock(n, false, func() { a.eachCodeItem(n, a.addJoinItem) })
}

// eachCodeItem walks a code block's body, invoking emit for each value-producing
// item in source order. Statement-style nodes that yield [expr.NoRef]
// (let-bindings, assignments, set/show rules) are skipped. Symmetric to
// [eachMarkupItem]; shared between [lowerCodeBlock] and any other code-block
// join scope.
func (a *analyzer) eachCodeItem(n syntax.Node, emit func(expr.Ref, syntax.Span)) {
	ns := a.inner(n, syntax.KindCodeBlock)
	entry := a.frame().pending
	for child := range ns.inside(syntax.KindLeftBrace, syntax.KindRightBrace) {
		if child.Kind() == syntax.KindCode {
			// A run of semicolon-separated statements; emit each in turn.
			stmts := a.inner(child, syntax.KindCode)
			for stmt := range stmts.all() {
				a.emitCodeItem(stmt, entry, emit)
			}
		} else {
			a.emitCodeItem(child, entry, emit)
		}
	}
}

// emitCodeItem lowers a single code statement and records its value item, if
// any. Once an escape is pending (relative to entry, the enclosing sequence's
// pending state), the statement can never execute: it is not lowered (Typst
// stops evaluating the sequence at the same point), only its parse errors are
// adopted.
func (a *analyzer) emitCodeItem(stmt syntax.Node, entry *pendingEscape, emit func(expr.Ref, syntax.Span)) {
	if a.frame().pending != entry {
		a.adoptParseErrors(stmt)
		return
	}
	switch stmt.Kind() {
	case syntax.KindError:
		emit(a.emitSyntaxError(stmt.(*syntax.Error)), stmt.Span())
	default:
		if ref := a.lowerExpr(stmt); ref != expr.NoRef {
			emit(ref, stmt.Span())
		}
	}
}

// lowerJoinedBlock lowers a block body as a join scope on the current frame:
// fill walks the body and records each value-producing item via
// [analyzer.addJoinItem], and the items are combined into a single value at
// completion. content selects the finalizer (a content sequence
// [expr.ContentResult] vs the code-mode joiner). The lexical scope must already
// be open.
//
// The scope exists so that a break/continue/return nested inside fill can join
// the values produced so far before it jumps. Blocks without an escape produce
// the same IR they would as a bare collect-and-join.
func (a *analyzer) lowerJoinedBlock(n syntax.Node, content bool, fill func()) expr.Ref {
	a.pushJoinScope(content)
	fill()
	return a.joinScopeValue(a.popJoinScope(), n.Span())
}

// Conditionals ///////////////////////////////////////////////////////////////

// lowerConditional lowers `if cond { ... } else if cond { ... } else { ... }`
// into a chain of basic blocks. Each arm writes a synthetic result
// variable; the join block reads it back, which inserts a block parameter
// automatically via the Builder's Braun construction.
//
// Whenever a sub-lowering (cond, body, or else-body) hands back [expr.NoRef]
// — typically because the parser substituted an [*syntax.Error] for that
// position — we route the current block through `jumpToJoin` instead of
// emitting an instruction that would dangle. The conditional's value is
// then None along the failing path; the recorded parser error already
// describes the underlying problem.
func (a *analyzer) lowerConditional(n syntax.Node) expr.Ref {
	// Surface embedded parser errors at the parent block before opening
	// any inner blocks. Walks the whole subtree since the parser nests
	// errors under Conditional children (e.g. "expected block" inside the
	// then-arm).
	a.collectChildErrors(n, true)

	join := a.b.NewBlock()
	result := a.b.NewVar(name.Make("$cond"))

	jumpToJoin := func(span syntax.Span) {
		none := a.b.Const(span, value.None{})
		a.b.WriteVar(result, a.b.CurrentBlock(), none)
		a.b.Jump(span, join)
	}

	var lower func(n syntax.Node)
	lower = func(n syntax.Node) {
		ns := a.inner(n, syntax.KindConditional)
		ns.take(syntax.KindIf)
		if ns.done() {
			// Parser failed before the condition was reached (e.g. bare
			// `#if`). The "expected expression" error is already emitted by
			// walkErrors above.
			jumpToJoin(n.Span())
			return
		}
		condNode := ns.node()
		if condNode.Kind() == syntax.KindError {
			// Condition is a parser error; the diagnostic is already at the
			// parent block, no need to branch on a placeholder value.
			jumpToJoin(n.Span())
			return
		}
		cond := a.lowerExpr(condNode)
		if cond == expr.NoRef {
			jumpToJoin(n.Span())
			return
		}
		thenBlk := a.b.NewBlock()
		elseBlk := a.b.NewBlock()
		a.b.Branch(condNode.Span(), cond, thenBlk, elseBlk)

		a.b.SetBlock(thenBlk)
		a.b.SealBlock(thenBlk)
		if ns.done() {
			// No body parsed; the parser's "expected block" diagnostic was
			// already emitted at the parent block.
			jumpToJoin(n.Span())
			a.b.SetBlock(elseBlk)
			a.b.SealBlock(elseBlk)
			jumpToJoin(n.Span())
			return
		}
		bodyNode := ns.node()
		if bodyNode.Kind() == syntax.KindError {
			// Error already emitted at the parent block by walkErrors.
			jumpToJoin(n.Span())
			a.b.SetBlock(elseBlk)
			a.b.SealBlock(elseBlk)
			jumpToJoin(n.Span())
			return
		}
		// Escapes fired by the catch transfer control on the branch's own
		// path, before it merges with the join block; the merge jump below
		// then lands in dead code.
		thenResult := a.catchEscapes(false, func() expr.Ref { return a.lowerBlock(bodyNode) })
		a.b.WriteVar(result, a.b.CurrentBlock(), thenResult)
		a.b.Jump(n.Span(), join)

		a.b.SetBlock(elseBlk)
		a.b.SealBlock(elseBlk)
		if !ns.at(syntax.KindElse) {
			jumpToJoin(n.Span())
			return
		}
		ns.node() // consume else
		if ns.done() {
			jumpToJoin(n.Span())
			return
		}
		if ns.at(syntax.KindConditional) {
			lower(ns.node())
			return
		}
		elseBodyNode := ns.node()
		if elseBodyNode.Kind() == syntax.KindError {
			// Error already emitted at the parent block by walkErrors.
			jumpToJoin(n.Span())
			return
		}
		elseResult := a.catchEscapes(false, func() expr.Ref { return a.lowerBlock(elseBodyNode) })
		a.b.WriteVar(result, a.b.CurrentBlock(), elseResult)
		a.b.Jump(n.Span(), join)
	}
	lower(n)

	a.b.SetBlock(join)
	a.b.SealBlock(join)
	return a.b.ReadVar(result, join)
}

// lowerBlock dispatches to code- or content-block lowering depending on the
// block kind. Used by control-flow constructs that take a block argument.
func (a *analyzer) lowerBlock(n syntax.Node) expr.Ref {
	switch n.Kind() {
	case syntax.KindCodeBlock:
		return a.lowerCodeBlock(n)
	case syntax.KindContentBlock:
		return a.lowerContentBlock(n)
	default:
		a.unexpected(n)
		return expr.NoRef
	}
}

// Loops //////////////////////////////////////////////////////////////////////

// lowerWhileLoop lowers `while cond { body }`.
func (a *analyzer) lowerWhileLoop(n syntax.Node) expr.Ref {
	// Detect error nodes early to avoid creating blocks that would be left
	// unterminated. Matches the pattern in lowerForLoop.
	if a.collectChildErrors(n, false) {
		return expr.NoRef
	}

	ns := a.inner(n, syntax.KindWhileLoop)
	ns.take(syntax.KindWhile)
	condNode := ns.node()
	bodyNode := ns.node()

	return a.lowerLoop(n,
		func() (expr.Ref, syntax.Span) { return a.lowerExpr(condNode), condNode.Span() },
		func() expr.Ref { return a.lowerBlock(bodyNode) })
}

// lowerForLoop lowers `for pattern in iterable { body }`.
func (a *analyzer) lowerForLoop(n syntax.Node) expr.Ref {
	if a.collectChildErrors(n, false) {
		return expr.NoRef
	}

	ns := a.inner(n, syntax.KindForLoop)
	ns.take(syntax.KindFor)
	patternNode := ns.node()
	ns.take(syntax.KindIn)
	iterableNode := ns.node()
	bodyNode := ns.node()

	iterable := a.lowerExpr(iterableNode)
	// Use the iterable expression's span so "cannot loop over X" errors
	// point at the iterable, not the whole `for` statement. When the
	// pattern is a destructuring pattern, IterOpen also refuses to iterate
	// strings (yielding "cannot destructure values of string" at the
	// pattern span instead).
	destructuring := patternNode.Kind() == syntax.KindDestructuring
	iter := a.b.IterOpen(iterableNode.Span(), iterable, destructuring, patternNode.Span())

	return a.lowerLoop(n,
		func() (expr.Ref, syntax.Span) { return a.b.IterHasNext(n.Span(), iter), n.Span() },
		func() expr.Ref {
			a.openScope()
			defer a.closeScope()
			elem := a.b.IterAdvance(n.Span(), iter)
			a.lowerDestructPattern(patternNode, elem)
			return a.lowerBlock(bodyNode)
		})
}

// lowerLoop builds the CFG scaffold shared by while and for loops: the join
// accumulator, the header/body/exit blocks, the escape catch, the back-edge
// tail, and the [expr.Builder.JoinResult] finalization that gives the loop
// expression its value at the exit block. cond emits the header's termination
// check and returns the branch condition with its span; body emits any
// per-iteration setup (lexical scope, element binding) and lowers the loop
// body, returning its value.
//
// The catch fires escapes that arose in the body — a break/continue reaching
// this loop, or a return passing through it; the regular tail then lands in
// dead code.
func (a *analyzer) lowerLoop(n syntax.Node, cond func() (expr.Ref, syntax.Span), body func() expr.Ref) expr.Ref {
	accName := a.b.NewVar(name.Make("$acc"))
	accInit := a.b.JoinBegin(n.Span())
	a.b.WriteVar(accName, a.b.CurrentBlock(), accInit)

	header := a.b.NewBlock()
	bodyBlk := a.b.NewBlock()
	exit := a.b.NewBlock()

	a.b.Jump(n.Span(), header)
	a.b.SetBlock(header)
	c, condSpan := cond()
	a.b.Branch(condSpan, c, bodyBlk, exit)

	a.b.SetBlock(bodyBlk)
	a.b.SealBlock(bodyBlk)
	a.pushLoop(header, exit, accName)
	bodyRef := a.catchEscapes(false, func() expr.Ref {
		ref := body()
		if ref == expr.NoRef {
			return a.b.Const(n.Span(), value.None{})
		}
		return ref
	})
	a.popLoop()
	a.appendJoin(accName, n.Span(), bodyRef)
	a.b.Jump(n.Span(), header)

	a.b.SealBlock(header)
	a.b.SetBlock(exit)
	a.b.SealBlock(exit)
	finalAcc := a.b.ReadVar(accName, exit)
	return a.b.JoinResult(n.Span(), finalAcc)
}

// lowerLoopBreak records a pending break escape (see [pendingEscape]). The
// control transfer is not emitted here: the statement containing the break is
// still evaluated to its end, and the escape fires at the innermost enclosing
// catch point. The break expression itself evaluates to none so enclosing
// expressions (call args, spreads, ...) have an operand.
func (a *analyzer) lowerLoopBreak(n syntax.Node) expr.Ref {
	if len(a.frame().loops) == 0 {
		return a.emitError(n.Span(), "cannot break outside of loop")
	}
	a.setPending(pendingEscape{kind: escapeBreak, span: n.Span()})
	return a.b.Const(n.Span(), value.None{})
}

// lowerLoopContinue records a pending continue escape; deferred delivery as in
// [lowerLoopBreak].
func (a *analyzer) lowerLoopContinue(n syntax.Node) expr.Ref {
	if len(a.frame().loops) == 0 {
		return a.emitError(n.Span(), "cannot continue outside of loop")
	}
	a.setPending(pendingEscape{kind: escapeContinue, span: n.Span()})
	return a.b.Const(n.Span(), value.None{})
}

// lowerFuncReturn records a pending return escape; deferred delivery as in
// [lowerLoopBreak]. `return x` returns x explicitly, discarding the function
// body's join; a bare `return` returns the body's joined-so-far value, both
// delivered by [analyzer.firePending]. `return` outside a function is an
// error.
func (a *analyzer) lowerFuncReturn(n syntax.Node) expr.Ref {
	if !a.frame().isFn {
		return a.emitError(n.Span(), "cannot return outside of function")
	}
	ns := a.inner(n, syntax.KindFuncReturn)
	ns.take(syntax.KindReturn)

	p := pendingEscape{kind: escapeReturn, span: n.Span(), val: expr.NoRef}
	if !ns.done() {
		p.val = a.lowerExpr(ns.node())
	}
	a.setPending(p)
	return a.b.Const(n.Span(), value.None{})
}

// Closures ///////////////////////////////////////////////////////////////////

// lowerClosure lowers a closure literal to a MakeClosure instruction. The
// closure body is constructed in a nested [expr.Function]; captures are
// allocated lazily as the body's lowerIdent calls encounter outer-scope
// names, then passed to MakeClosure once the body is fully lowered.
func (a *analyzer) lowerClosure(n syntax.Node) expr.Ref {
	return a.lowerClosureNamed(n, name.Invalid)
}

// lowerClosureNamed lowers a closure literal. recName, when non-invalid, is the
// closure's name and is bound inside the body as a self-reference Ref (so direct
// recursion needs no capture) and used as the function's display name. It is set
// only for the `let f(x) = body` function-shorthand form, where `f` is in scope
// within its own body — not for `let f = (x) => body`, where the body's `f`
// resolves to the outer binding and the closure is anonymous.
func (a *analyzer) lowerClosureNamed(n syntax.Node, recName name.Name) expr.Ref {
	// Emit any structural parser errors (e.g. missing `=`, missing body)
	// at the OUTER block first. Errors deep inside the closure's body live
	// in the closure's IR frame and only fire when the closure is called;
	// structural errors at the closure's top level describe the definition
	// itself and should fire eagerly. Unlike loops/conditionals we don't
	// bail on structural errors — the closure body may still be valid IR.
	a.collectChildErrors(n, false)

	ns := a.inner(n, syntax.KindClosure)

	// Parse closure header — peek at the first node to determine form.
	closureName := recName
	var paramsNode syntax.Node
	switch n0 := ns.node(); n0.Kind() {
	case syntax.KindIdent:
		if ns.at(syntax.KindParams) {
			closureName = name.Make(a.str(n0))
			paramsNode = ns.node()
		} else {
			// Single-arg form: `x => body`. Treat ident as a positional param
			// in a synthetic params node.
			paramsNode = n0
		}
	case syntax.KindUnderscore:
		paramsNode = n0
	case syntax.KindParams:
		paramsNode = n0
	default:
		panic("invalid closure syntax: " + n0.Kind().String())
	}

	// Consume arrow or eq.
	if ns.at(syntax.KindArrow) || ns.at(syntax.KindEq) {
		ns.node()
	}
	if ns.done() {
		a.expected(&ns, "expression")
		return expr.NoRef
	}
	bodyNode := ns.node()

	// Lower named-parameter defaults in the OUTER scope so identifiers in
	// them resolve to outer bindings (Typst semantics: defaults are
	// captured at closure construction time, not re-evaluated at call
	// time in the closure's scope).
	paramSpecs := a.collectClosureParamSpecs(paramsNode)
	defaultRefs := make([]expr.Ref, len(paramSpecs))
	for i, p := range paramSpecs {
		if p.kind == expr.ParamNamed && p.defaultNode != nil {
			defaultRefs[i] = a.lowerExpr(p.defaultNode)
		} else {
			defaultRefs[i] = expr.NoRef
		}
	}

	// Begin nested function: push a frame with its own builder and boundary
	// scope. [a.b] and [a.scope] now refer to the new frame.
	a.pushFrame()
	if closureName != name.Invalid {
		a.b.Function().Name = closureName.String()
	}

	// Bind the recursion name (if any) as a self-binding. The actual
	// The self Ref is allocated lazily by [resolveName] only if the body
	// references the name, so non-recursive functions stay capture-free and
	// preserve their existing SSA shape.
	if recName != name.Invalid {
		if a.scope.bindings == nil {
			a.scope.bindings = make(map[name.Name]binding)
		}
		a.scope.bindings[recName] = selfBinding{}
	}

	// Register parameters, capturing outer-scope default values. Defaults
	// are added as captures first, so the [Function.Captures] slice has
	// defaults at the front (in declaration order) and body-captured names
	// afterwards. [MakeClosure]'s capture list mirrors this order.
	var defaultOuterRefs []expr.Ref // outer Refs to thread into MakeClosure
	for i, p := range paramSpecs {
		innerDefault := expr.NoRef
		if p.kind == expr.ParamNamed && defaultRefs[i] != expr.NoRef {
			innerDefault = a.b.AddCapture(name.Make("$default"), p.span)
			defaultOuterRefs = append(defaultOuterRefs, defaultRefs[i])
		}
		switch p.kind {
		case expr.ParamPositional:
			switch {
			case p.destructure != nil:
				paramRef := a.b.AddParam(underscore, expr.ParamPositional, expr.NoRef, p.span)
				a.lowerDestructPattern(p.destructure, paramRef)
			case p.name != name.Invalid:

				v := a.allocVar(p.name)
				paramRef := a.b.AddParam(p.name, expr.ParamPositional, expr.NoRef, p.span)
				a.b.WriteVar(v, expr.BlockID(0), paramRef)
			default:
				a.b.AddParam(underscore, expr.ParamPositional, expr.NoRef, p.span)
			}
		case expr.ParamNamed:

			v := a.allocVar(p.name)
			paramRef := a.b.AddParam(p.name, expr.ParamNamed, innerDefault, p.span)
			a.b.WriteVar(v, expr.BlockID(0), paramRef)
		case expr.ParamSink:
			if p.name != name.Invalid {

				v := a.allocVar(p.name)
				paramRef := a.b.AddParam(p.name, expr.ParamSink, expr.NoRef, p.span)
				a.b.WriteVar(v, expr.BlockID(0), paramRef)
			} else {
				a.b.AddParam(underscore, expr.ParamSink, expr.NoRef, p.span)
			}
		}
	}

	// Lower the body. The frame's root region (seeded as [regionFnBody] by
	// pushFrame) is the return target: a nested bare `return` resolves there and
	// yields the body's partial join (Typst's joining `return` semantics). A
	// block body opens its join scope on that region; an expression body opens
	// none, so a bare `return` there yields none.
	// This frame is the return target, so an escape caught here is
	// necessarily a return, and this is the one catch where an explicit value
	// provably discards the body join on every call — hence the discard
	// warning for code-block bodies (markup bodies join naturally and never
	// warn). The fallthrough Return lands in dead code when the catch fires.
	warnDiscard := bodyNode.Kind() == syntax.KindCodeBlock
	bodyRef := a.catchEscapes(warnDiscard, func() expr.Ref { return a.lowerBlockOrExpr(bodyNode) })
	a.b.Return(n.Span(), bodyRef)
	a.b.Finalize()

	// Snapshot the inner builder's ordered list of capture source names
	// before tearing it down; the IR doesn't carry names, so the analyzer
	// queries the builder directly.
	captures := a.b.Captures()
	innerFn := a.b.Function()
	a.popFrame()
	funcID := a.mb.RegisterFunction(innerFn)

	// Build the capture Ref list in outer scope. The first
	// len(defaultOuterRefs) entries are the default-value captures (in
	// declaration order); the remainder are body-referenced captures
	// resolved by source name.
	captureRefs := make([]expr.Ref, len(captures))
	copy(captureRefs, defaultOuterRefs)
	for i := len(defaultOuterRefs); i < len(captures); i++ {
		captureRefs[i] = a.resolveName(captures[i], n.Span())
	}
	return a.b.MakeClosure(n.Span(), funcID, captureRefs)
}

// closureParamSpec describes one parameter as parsed by [collectClosureParamSpecs].
type closureParamSpec struct {
	name        name.Name
	kind        expr.ParamKind
	defaultNode syntax.Node // valid for ParamNamed
	span        syntax.Span // span of the parameter declaration (including default)
	nameSpan    syntax.Span // span of just the name; used for duplicate-name errors
	// destructure, when non-nil, is a destructuring pattern (KindDestructuring)
	// that should bind names from the positional argument bound to this
	// param. The param itself is anonymous from the caller's perspective.
	destructure syntax.Node
}

// collectClosureParamSpecs walks a Params (or single-ident) node and returns
// the param descriptors. Defaults are returned as syntax nodes to be lowered
// later by the caller (in whatever scope makes sense).
func (a *analyzer) collectClosureParamSpecs(n syntax.Node) []closureParamSpec {
	var out []closureParamSpec
	seen := make(map[name.Name]struct{})
	addName := func(n name.Name, span syntax.Span) {
		if n == name.Invalid {
			return
		}
		if _, dup := seen[n]; dup {
			a.emitError(span, "duplicate parameter: "+n.String())
		}
		seen[n] = struct{}{}
	}
	sawSink := false
	add := func(s closureParamSpec) {
		ns := s.nameSpan
		if ns == (syntax.Span{}) {
			ns = s.span
		}
		if s.kind == expr.ParamSink {
			if sawSink {
				a.emitError(s.span, "only one argument sink is allowed")
			}
			sawSink = true
		}
		addName(s.name, ns)
		out = append(out, s)
	}
	switch n.Kind() {
	case syntax.KindIdent:
		add(closureParamSpec{name: name.Make(a.leaf(n, syntax.KindIdent)), kind: expr.ParamPositional, span: n.Span(), nameSpan: n.Span()})
		return out
	case syntax.KindUnderscore:
		add(closureParamSpec{kind: expr.ParamPositional, span: n.Span()})
		return out
	}
	ns := a.inner(n, syntax.KindParams)
	if !ns.at(syntax.KindLeftParen) {
		for child := range ns.all() {
			a.collectClosureParamChild(child, add)
		}
		return out
	}
	for child := range ns.inside(syntax.KindLeftParen, syntax.KindRightParen) {
		if child.Kind() == syntax.KindComma {
			continue
		}
		a.collectClosureParamChild(child, add)
	}
	return out
}

func (a *analyzer) collectClosureParamChild(child syntax.Node, add func(closureParamSpec)) {
	switch child.Kind() {
	case syntax.KindIdent:
		add(closureParamSpec{name: name.Make(a.leaf(child, syntax.KindIdent)), kind: expr.ParamPositional, span: child.Span(), nameSpan: child.Span()})
	case syntax.KindUnderscore:
		add(closureParamSpec{kind: expr.ParamPositional, span: child.Span()})
	case syntax.KindNamed:
		named := a.inner(child, syntax.KindNamed)
		nameNode := named.node()
		if nameNode.Kind() != syntax.KindIdent {
			// Parser substituted an error (or keyword) for the param
			// name — record it and skip the whole named param. Don't
			// lower the default either: doing so resolves the
			// would-be-value as an outer-scope name and produces a
			// misleading cascading "unknown variable" diagnostic.
			a.unexpected(nameNode)

			return
		}
		source := name.Make(a.leaf(nameNode, syntax.KindIdent))
		named.take(syntax.KindColon)
		defaultNode := named.node()

		add(closureParamSpec{name: source, kind: expr.ParamNamed, defaultNode: defaultNode, span: child.Span(), nameSpan: nameNode.Span()})
	case syntax.KindSpread:
		spread := a.inner(child, syntax.KindSpread)
		spread.take(syntax.KindDots)
		var sinkName name.Name
		var sinkSpan syntax.Span
		if !spread.done() {
			identNode := spread.node()
			sinkName = name.Make(a.leaf(identNode, syntax.KindIdent))
			sinkSpan = identNode.Span()
		}

		add(closureParamSpec{name: sinkName, kind: expr.ParamSink, span: child.Span(), nameSpan: sinkSpan})
	case syntax.KindError:
		a.emitSyntaxError(child.(*syntax.Error))
	case syntax.KindDestructuring, syntax.KindParenthesized:
		// Destructuring parameter: bind names from the destructure pattern
		// against the corresponding positional arg. The caller-visible
		// param itself is anonymous.
		add(closureParamSpec{kind: expr.ParamPositional, span: child.Span(), destructure: child})
	default:
		a.emitError(child.Span(), "unexpected parameter: "+child.Kind().Name())
	}
}

// lowerBlockOrExpr lowers a closure body, which is either a block or a
// single expression.
func (a *analyzer) lowerBlockOrExpr(n syntax.Node) expr.Ref {
	switch n.Kind() {
	case syntax.KindCodeBlock, syntax.KindContentBlock:
		return a.lowerBlock(n)
	default:
		return a.lowerExpr(n)
	}
}

// closureSourceName peeks at the children of a KindClosure node and returns
// the function name if this is the named form (`name(params) = body`).
func (a *analyzer) closureSourceName(n syntax.Node) (name.Name, bool) {
	inner, ok := n.(*syntax.Inner)
	if !ok {
		return name.Invalid, false
	}
	children := inner.Children()
	if len(children) >= 2 && children[0].Kind() == syntax.KindIdent && children[1].Kind() == syntax.KindParams {
		return name.Make(a.str(children[0])), true
	}
	return name.Invalid, false
}

// Markup constructs //////////////////////////////////////////////////////////

func (a *analyzer) lowerHeading(n syntax.Node) expr.Ref {
	ns := a.inner(n, syntax.KindHeading)
	marker, ok := ns.take(syntax.KindHeadingMarker)
	if !ok {
		return expr.NoRef
	}
	body := a.lowerMarkup(ns.node())
	return a.b.Heading(n.Span(), len(marker), body)
}

func (a *analyzer) lowerStrong(n syntax.Node) expr.Ref {
	ns := a.inner(n, syntax.KindStrong)
	ns.takeDelim(syntax.KindStar)
	if ns.done() {
		return expr.NoRef
	}
	body := a.lowerMarkup(ns.node())
	ns.takeDelim(syntax.KindStar)
	return a.b.Strong(n.Span(), body)
}

func (a *analyzer) lowerEmph(n syntax.Node) expr.Ref {
	ns := a.inner(n, syntax.KindEmph)
	ns.takeDelim(syntax.KindUnderscore)
	if ns.done() {
		return expr.NoRef
	}
	body := a.lowerMarkup(ns.node())
	ns.takeDelim(syntax.KindUnderscore)
	return a.b.Emph(n.Span(), body)
}

func (a *analyzer) lowerRef(n syntax.Node) expr.Ref {
	ns := a.inner(n, syntax.KindRef)
	marker, ok := ns.take(syntax.KindRefMarker)
	if !ok {
		return expr.NoRef
	}
	target := name.Make(marker[1:])
	supplement := expr.NoRef
	if !ns.done() {
		supplement = a.lowerContentBlock(ns.node())
	}
	return a.b.RefMarkup(n.Span(), target, supplement)
}

func (a *analyzer) lowerListItem(n syntax.Node) expr.Ref {
	ns := a.inner(n, syntax.KindListItem)
	ns.take(syntax.KindListMarker)
	body := a.lowerMarkup(ns.node())
	return a.b.ListItem(n.Span(), body)
}

func (a *analyzer) lowerEnumItem(n syntax.Node) expr.Ref {
	ns := a.inner(n, syntax.KindEnumItem)
	number := -1
	marker, ok := ns.take(syntax.KindEnumMarker)
	if !ok {
		return expr.NoRef
	}
	if marker != "+" {
		v, err := strconv.ParseInt(strings.TrimSuffix(marker, "."), 10, 64)
		if err != nil {
			panic("invalid enum marker: " + marker)
		}
		number = int(v)
	}
	body := a.lowerMarkup(ns.node())
	return a.b.EnumItem(n.Span(), number, body)
}

func (a *analyzer) lowerTermItem(n syntax.Node) expr.Ref {
	ns := a.inner(n, syntax.KindTermItem)
	ns.take(syntax.KindTermMarker)
	term := a.lowerMarkup(ns.node())
	ns.take(syntax.KindColon)
	desc := a.lowerMarkup(ns.node())
	return a.b.TermItem(n.Span(), term, desc)
}

func (a *analyzer) lowerContentBlock(n syntax.Node) expr.Ref {
	ns := a.inner(n, syntax.KindContentBlock)
	a.openScope()
	defer a.closeScope()
	// A content block's body is markup even when it appears inside an equation,
	// so escapes in it are text, not math symbols.
	defer func(d int) { a.mathDepth = d }(a.mathDepth)
	a.mathDepth = 0
	ns.take(syntax.KindLeftBracket)
	bodyNode := ns.node()
	// The closing bracket may be absent when the parser is recovering from
	// an unclosed delimiter — the "unclosed delimiter" error is already on
	// the opening `[`, so don't escalate to internal() here.
	if ns.at(syntax.KindRightBracket) {
		ns.take(syntax.KindRightBracket)
	}
	return a.lowerJoinedBlock(bodyNode, true, func() { a.eachMarkupItem(bodyNode, a.addJoinItem) })
}

func (a *analyzer) lowerRaw(n syntax.Node) expr.Ref {
	ns := a.inner(n, syntax.KindRaw)
	marker, ok := ns.take(syntax.KindRawDelim)
	if !ok {
		return expr.NoRef
	}
	var lang string
	if ns.at(syntax.KindRawLang) {
		lang, _ = ns.take(syntax.KindRawLang)
	}

	block := marker != "`"
	var sb strings.Builder
	first := true
	for child := range ns.all() {
		switch child.Kind() {
		case syntax.KindRawDelim:
		case syntax.KindRawTrimmed:
			continue
		default:
			if child.Kind() != syntax.KindText {
				panic("invalid node kind in raw: " + child.Kind().String())
			}
			// Each KindText child is a single line; the line breaks
			// between them are dropped by the scanner (RawTrimmed),
			// so rejoin lines with a newline separator (no trailing
			// newline) for both inline and block raw.
			if !first {
				sb.WriteByte('\n')
			}
			sb.WriteString(a.str(child))
			first = false
		}
	}
	return a.b.Const(n.Span(), &value.Raw{Block: block, Lang: lang, Text: sb.String()})
}

// Set/show/contextual/include stubs //////////////////////////////////////////
//
// These constructs have placeholder instructions in the IR; the evaluator
// panics on them today and will continue to do so until they're implemented
// in their own design pass.

func (a *analyzer) lowerSetRule(n syntax.Node) expr.Ref {
	ns := a.inner(n, syntax.KindSetRule)
	ns.take(syntax.KindSet)
	if ns.done() {
		a.expected(&ns, "expression")
		return expr.NoRef
	}
	target := a.lowerExpr(ns.node())
	// `parseArgs` bails without wrapping an Args node when the target is not
	// directly followed by a delimiter, so a set rule can lack its argument
	// list entirely (`#set .A`, `#(set!if`).
	if !ns.at(syntax.KindArgs) {
		if ns.at(syntax.KindError) {
			a.unexpected(ns.node())
		} else {
			a.expected(&ns, "argument list")
		}
		return expr.NoRef
	}
	args, _ := a.lowerArgs(ns.node())
	cond := expr.NoRef
	if ns.at(syntax.KindIf) {
		ns.node()
		if ns.done() {
			a.expected(&ns, "expression")
			return expr.NoRef
		}
		cond = a.lowerExpr(ns.node())
	}
	return a.b.SetRule(n.Span(), target, args, cond)
}

func (a *analyzer) lowerShowRule(n syntax.Node) expr.Ref {
	ns := a.inner(n, syntax.KindShowRule)
	ns.take(syntax.KindShow)
	selector := expr.NoRef
	if !ns.at(syntax.KindColon) {
		if ns.done() {
			a.expected(&ns, "selector")
			return expr.NoRef
		}
		selector = a.lowerExpr(ns.node())
	}
	ns.take(syntax.KindColon)
	if ns.done() {
		a.expected(&ns, "expression")
		return expr.NoRef
	}
	transform := a.lowerExpr(ns.node())
	return a.b.ShowRule(n.Span(), selector, transform)
}

func (a *analyzer) lowerContextual(n syntax.Node) expr.Ref {
	ns := a.inner(n, syntax.KindContextual)
	ns.take(syntax.KindContext)
	body := a.lowerExpr(ns.node())
	return a.b.Contextual(n.Span(), body)
}

// Destructure-assignment ////////////////////////////////////////////////////

// lowerDestructAssignment lowers `(a, b) = expr` — assignment to existing
// bindings. The RHS is computed, then each pattern leaf re-assigns the
// existing binding via the same lookup path as plain `x = rhs`.
func (a *analyzer) lowerDestructAssignment(n syntax.Node) expr.Ref {
	ns := a.inner(n, syntax.KindDestructAssignment)
	patternNode := ns.node()
	ns.take(syntax.KindEq)
	rhs := a.lowerExpr(ns.node())
	a.assignDestructPattern(patternNode, rhs)
	// Destructuring assignment is a statement: it produces no value and
	// no surrounding content (parbreaks etc. should not be emitted for
	// it). Returning NoRef matches lowerLetBinding's behaviour.
	return expr.NoRef
}

func (a *analyzer) lowerModuleInclude(n syntax.Node) expr.Ref {
	ns := a.inner(n, syntax.KindModuleInclude)
	ns.take(syntax.KindInclude)
	source := a.lowerExpr(ns.node())
	return a.b.ModuleInclude(n.Span(), source)
}
