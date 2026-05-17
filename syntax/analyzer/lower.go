package analyzer

import (
	"errors"
	"math"
	"strconv"
	"strings"

	"znkr.io/writst/expr"
	"znkr.io/writst/name"
	"znkr.io/writst/syntax"
	"znkr.io/writst/syntax/convert"
	"znkr.io/writst/value"
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
	ns := a.inner(n, syntax.KindMarkup)
	var items []expr.Ref
	for child := range ns.all() {
		switch child.Kind() {
		case syntax.KindSemicolon, syntax.KindHash, syntax.KindSpace:
			continue
		case syntax.KindError:
			items = append(items, a.emitSyntaxError(child.(*syntax.Error)))
		case syntax.KindLabel:
			if len(items) == 0 {
				// Detached label: drop it. (Matches legacy "no preceding
				// content" behavior.)
				continue
			}
			label := a.leaf(child, syntax.KindLabel)
			labelName := name.Make(label[1 : len(label)-1])
			a.b.AttachLabel(child.Span(), items[len(items)-1], labelName)
		default:
			ref := a.lowerExpr(child)
			if ref != expr.NoRef {
				items = append(items, ref)
			}
		}
	}
	return items
}

// lowerExpr is the dispatch entry for converting a single code- or markup-
// mode syntax node into an SSA value reference. Statement-style nodes
// (let-bindings, set/show rules, assignments) return [expr.NoRef] to
// indicate they shouldn't contribute to a content sequence.
func (a *analyzer) lowerExpr(n syntax.Node) expr.Ref {
	switch n.Kind() {
	// Markup leaves
	case syntax.KindText:
		return a.b.Const(n.Span(), &value.Text{Text: strings.TrimSpace(n.Text())})
	case syntax.KindEscape:
		return a.b.Const(n.Span(), &value.Text{Text: unescape(n.Text())})
	case syntax.KindShorthand:
		return a.b.Const(n.Span(), &value.Text{Text: unshorthand(n.Text())})
	case syntax.KindSmartQuote:
		return a.b.Const(n.Span(), &value.Text{Text: n.Text()})
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
		lit := n.Text()
		body := a.b.Const(n.Span(), &value.Text{Text: lit})
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
	case syntax.KindError:
		return a.emitSyntaxError(n.(*syntax.Error))
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

func (a *analyzer) lowerNumeric(n syntax.Node) expr.Ref {
	val := a.leaf(n, syntax.KindNumeric)
	idx := strings.IndexFunc(val, func(r rune) bool { return (r < '0' || r > '9') && r != '.' })
	if idx == -1 {
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
// resolved value; self-binding (recursion name) at depth 0 → [expr.DefSelf]
// in the current builder; local (no boundary crossed) → ReadVar with the
// SSA variable; outer (one or more boundaries crossed) → allocate a
// capture in the innermost active closure. Transitive captures (the binding
// sits outside more than one closure) work because lowerClosure resolves
// each capture's outer Ref in its enclosing builder via resolveName, which
// recurses through any intermediate closures.
func (a *analyzer) resolveName(source name.Name, span syntax.Span) expr.Ref {
	frameIdx := len(a.frames) - 1
	inCurrent := true
	for s := a.scope; s != nil; s = s.parent {
		if binding, ok := s.bindings[source]; ok {
			switch {
			case binding.value != nil:
				return a.b.Const(span, binding.value)
			case binding.self && inCurrent:
				return a.b.Self(span)
			case inCurrent:
				return a.b.ReadVar(binding.variable, a.b.CurrentBlock())
			default:
				// Found the name outside the current closure.
				f := a.frames[frameIdx]
				if ref, ok := f.b.PeekVar(binding.variable); ok && ref.IsModConst() {
					// Reference is a module-constant, which is immutable which
					// doesn't require a capture.
					return ref
				}
				// Fall back to capturing the variable in the current closure.
				return a.captureRef(source, span)
			}
		}
		if s == a.frames[frameIdx].scope {
			inCurrent = false
			frameIdx--
		}
	}
	// checkIdent already reported "unknown variable" — this is a bug if we
	// get here, but we don't want to crash silently.
	return expr.NoRef
}

// captureRef returns the [expr.DefCapture] Ref for source in the current
// frame, allocating one on first reference. Captures are never reassignable,
// so no SSA-variable indirection is needed — the returned Ref is the
// canonical value for source in this frame.
func (a *analyzer) captureRef(source name.Name, span syntax.Span) expr.Ref {
	f := a.frames[len(a.frames)-1]
	if ref, ok := f.captures[source]; ok {
		return ref
	}
	ref := a.b.AddCapture(expr.Var{Name: source}, span)
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
	boundary := a.frames[len(a.frames)-1].scope
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
		source := name.Make(a.leaf(leftNode, syntax.KindIdent))
		binding, ok := a.lookup(source)
		if !ok {
			// Either unknown or only known as a builtin. Use checkIdent's
			// existing message for unknowns; for builtins we need a more
			// specific error since checkIdent would pass them.
			return a.checkIdent(source, leftNode.Span())
		}
		var newVal expr.Ref
		if op == syntax.Assign {
			newVal = a.lowerExpr(rightNode)
		} else {
			// Match Typst semantics: RHS is computed first (may shadow the LHS
			// via side-effects), then the LHS's current value is read, then
			// combined with the RHS.
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
			rhs := a.lowerExpr(rightNode)
			// Re-resolve in case the RHS shadowed the binding via a `let`.
			if b, ok := a.lookup(source); ok {
				binding = b
			}
			old := a.b.ReadVar(binding.variable, a.b.CurrentBlock())
			newVal = a.b.Binary(span, op.StripAssign(), old, rhs)
		}
		a.b.WriteVar(binding.variable, a.b.CurrentBlock(), newVal)
		return a.b.Const(span, value.None{})
	case syntax.KindParenthesized:
		// Unwrap and recurse.
		ns := a.inner(leftNode, syntax.KindParenthesized)
		var inner syntax.Node
		for child := range ns.inside(syntax.KindLeftParen, syntax.KindRightParen) {
			inner = child
		}
		return a.lowerAssign(span, op, inner, rightNode)
	case syntax.KindFieldAccess:
		// `x.f = v` → FieldWrite{x, f, v}; `x.f += v` carries op.
		if baseNode, ok := lvalueBase(leftNode); ok {
			source := name.Make(baseNode.Text())
			if a.isCapturedVar(source) {
				return a.emitError(baseNode.Span(), "variables from outside the function are read-only and cannot be modified")
			}
		}
		fns := a.inner(leftNode, syntax.KindFieldAccess)
		target := a.lowerExpr(fns.node())
		fns.take(syntax.KindDot)
		fieldName := name.Make(a.leaf(fns.node(), syntax.KindIdent))
		newVal := a.lowerExpr(rightNode)
		stripped := op
		if op != syntax.Assign {
			stripped = op.StripAssign()
		}
		// Use the LHS span for the FieldWrite so runtime errors point at the
		// field-access expression, not the whole assignment.
		a.b.FieldWrite(leftNode.Span(), target, fieldName, newVal, stripped)
		return a.b.Const(span, value.None{})
	case syntax.KindFuncCall:
		// `f(args) = v` (incl. method-call form like `a.at(i) = v`) → CallSet.
		if baseNode, ok := lvalueBase(leftNode); ok {
			source := name.Make(baseNode.Text())
			if a.isCapturedVar(source) {
				return a.emitError(baseNode.Span(), "variables from outside the function are read-only and cannot be modified")
			}
		}
		fns := a.inner(leftNode, syntax.KindFuncCall)
		callee := a.lowerExpr(fns.node())
		args, blocks := a.lowerArgs(fns.node())
		newVal := a.lowerExpr(rightNode)
		stripped := op
		if op != syntax.Assign {
			stripped = op.StripAssign()
		}
		// Use the LHS span so runtime errors point at the call expression.
		a.b.CallSet(leftNode.Span(), callee, args, blocks, newVal, stripped)
		return a.b.Const(span, value.None{})
	case syntax.KindBinary, syntax.KindUnary:
		// Genuinely-temporary lvalues (e.g. `(1+2) = 3`). Lower the LHS so
		// any runtime errors fire first (matching legacy where the eval
		// error of the LHS supersedes "cannot mutate"); then emit a deferred
		// Error wired to the LHS so the diagnostic only fires when the LHS
		// itself didn't already error.
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
	seen := make(map[string]bool)
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
			if seen[key] {
				a.emitError(child.Span(), "duplicate key: "+key)
			}
			seen[key] = true
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
				if leaf, ok := peekLeafText(entry); ok {
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
				if seen[keyStr] {
					a.emitError(child.Span(), "duplicate key: "+keyStr)
				}
				seen[keyStr] = true
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

// peekAfterEq returns the kind of the first non-trivia node after the `=` at
// the cursor's current position, or [syntax.KindNone] if none exists. The
// cursor is left untouched.
func peekAfterEq(ns *nodes) syntax.Kind {
	for i := ns.pos + 1; i < len(ns.items); i++ {
		k := ns.items[i].Kind()
		if k == syntax.KindSpace || k == syntax.KindLineComment || k == syntax.KindBlockComment {
			continue
		}
		return k
	}
	return syntax.KindNone
}

// peekLeafText returns the text of the current node if it is a *syntax.Leaf.
// Used to peek at string-literal keys for compile-time duplicate detection
// without consuming the node.
func peekLeafText(ns *nodes) (string, bool) {
	if ns.pos >= len(ns.items) {
		return "", false
	}
	if leaf, ok := ns.items[ns.pos].(*syntax.Leaf); ok {
		return leaf.Text(), true
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

// Function calls //////////////////////////////////////////////////////////////

func (a *analyzer) lowerFuncCall(n syntax.Node) expr.Ref {
	ns := a.inner(n, syntax.KindFuncCall)
	callee := a.lowerExpr(ns.node())
	args, blocks := a.lowerArgs(ns.node())
	return a.b.Call(n.Span(), callee, args, blocks, false)
}

func (a *analyzer) lowerArgs(n syntax.Node) ([]expr.CallArg, []expr.Ref) {
	ns := a.inner(n, syntax.KindArgs)

	var args []expr.CallArg
	seen := make(map[string]bool)
	if ns.at(syntax.KindLeftParen) {
		for child := range ns.inside(syntax.KindLeftParen, syntax.KindRightParen) {
			switch child.Kind() {
			case syntax.KindComma:
				continue
			case syntax.KindSpread:
				args = append(args, expr.CallArg{Kind: expr.ArgSpread, Value: a.lowerSpread(child), Span: child.Span()})
			case syntax.KindNamed:
				named := a.inner(child, syntax.KindNamed)
				keyText, ok := named.take(syntax.KindIdent)
				if !ok {
					continue
				}
				key := name.Make(keyText)
				named.take(syntax.KindColon)
				valNode := named.node()
				val := a.lowerExpr(valNode)

				if seen[key.String()] {
					a.emitError(child.Span(), "duplicate argument: "+key.String())
				}
				seen[key.String()] = true
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
		recName, ok := closureSourceName(closureNode)
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
	// `let f = (..) => body` form: pass f as the recursion name so the body
	// can refer to itself via a DefSelf binding.
	if patternNode.Kind() == syntax.KindIdent && ns.at(syntax.KindEq) && peekAfterEq(ns) == syntax.KindClosure {
		recName := name.Make(a.leaf(patternNode, syntax.KindIdent))
		ns.node() // consume eq
		closureNode := ns.node()
		rhs := a.lowerClosureNamed(closureNode, recName)
		v := a.allocVar(recName)
		a.b.WriteVar(v, a.b.CurrentBlock(), rhs)
		return expr.NoRef
	}
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

// lowerDestructPattern binds names from a destructuring pattern to portions
// of the RHS, emitting Extract / LengthCheck instructions as needed. Each
// binding allocates a fresh SSA variable in the current scope.
//
// Single-ident parenthesised patterns (e.g. `let (a) = ...`) are treated as
// grouping rather than destructuring: the lone name binds to the whole RHS.
// This matches the legacy semantics used by the integration tests.
func (a *analyzer) lowerDestructPattern(n syntax.Node, rhs expr.Ref) {
	switch n.Kind() {
	case syntax.KindIdent:
		source := name.Make(a.leaf(n, syntax.KindIdent))

		v := a.allocVar(source)
		a.b.WriteVar(v, a.b.CurrentBlock(), rhs)
		return
	case syntax.KindUnderscore:
		return
	}

	ns := a.inner(n, syntax.KindDestructuring)
	type binding struct {
		variable expr.Var
		span     syntax.Span
		idx      int
	}
	var bindings []binding
	hasSink := false
	idx := 0
	holes := 0
	hasComplex := false
	hasErrors := false
	seenNames := make(map[name.Name]bool)
	for child := range ns.inside(syntax.KindLeftParen, syntax.KindRightParen) {
		switch child.Kind() {
		case syntax.KindComma:
			continue
		case syntax.KindError:
			hasComplex = true
			hasErrors = true
			a.emitSyntaxError(child.(*syntax.Error))
		case syntax.KindUnderscore:
			holes++
			idx++
		case syntax.KindIdent:
			source := name.Make(a.leaf(child, syntax.KindIdent))
			if seenNames[source] {
				a.emitError(child.Span(), "duplicate binding: "+source.String())
			}
			seenNames[source] = true

			v := a.allocVar(source)
			bindings = append(bindings, binding{variable: v, span: child.Span(), idx: idx})
			idx++
		case syntax.KindSpread:
			hasComplex = true
			if hasSink {
				a.emitError(child.Span(), "only one destructuring sink is allowed")
			}
			hasSink = true
			spread := a.inner(child, syntax.KindSpread)
			spread.take(syntax.KindDots)
			if !spread.done() {
				inner := spread.node()
				switch inner.Kind() {
				case syntax.KindIdent, syntax.KindUnderscore:
					// Sink pattern accepted at analyze time; runtime is a
					// future TODO. Don't emit an analyze-time error.
				case syntax.KindError:
					hasErrors = true
					a.emitSyntaxError(inner.(*syntax.Error))
				default:
					hasErrors = true
					a.emitError(inner.Span(), "expected pattern, found "+inner.Kind().Name())
				}
			}

		case syntax.KindNamed:
			hasComplex = true
			named := a.inner(child, syntax.KindNamed)
			named.node() // name
			named.take(syntax.KindColon)
			patNode := named.node()

			switch patNode.Kind() {
			case syntax.KindIdent, syntax.KindUnderscore:
				hasErrors = true
				a.emitError(child.Span(), "ssa lowering of named destructuring patterns not yet implemented")
			case syntax.KindError:
				hasErrors = true
				a.emitSyntaxError(patNode.(*syntax.Error))
			default:
				hasErrors = true
				a.emitError(patNode.Span(), "expected pattern, found "+patNode.Kind().Name())
			}
		}
	}
	// `let (a) = rhs` (one ident, no holes, no sink/named): treat parens as
	// grouping and bind a to the whole RHS — matches legacy semantics.
	if !hasComplex && holes == 0 && len(bindings) == 1 && idx == 1 {
		a.b.WriteVar(bindings[0].variable, a.b.CurrentBlock(), rhs)
		return
	}
	// Pattern already contains errors; skip the runtime LengthCheck/Extract
	// emissions to avoid cascading "cannot destructure" diagnostics on top
	// of the underlying problem.
	if hasErrors {
		return
	}
	a.b.LengthCheck(n.Span(), rhs, idx, hasSink)
	for _, b := range bindings {
		ref := a.b.Extract(b.span, rhs, b.idx)
		a.b.WriteVar(b.variable, a.b.CurrentBlock(), ref)
	}
}

// Code blocks ////////////////////////////////////////////////////////////////

// lowerCodeBlock opens a new scope, lowers all statements inline into the
// current basic block (code blocks introduce no control flow on their own),
// and joins their values using the code-mode joiner.
func (a *analyzer) lowerCodeBlock(n syntax.Node) expr.Ref {
	ns := a.inner(n, syntax.KindCodeBlock)
	a.openScope()
	defer a.closeScope()
	var items []expr.Ref
	var spans []syntax.Span
	for child := range ns.inside(syntax.KindLeftBrace, syntax.KindRightBrace) {
		switch child.Kind() {
		case syntax.KindCode:
			items, spans = a.lowerCodeInto(child, items, spans)
		case syntax.KindError:
			items = append(items, a.emitSyntaxError(child.(*syntax.Error)))
			spans = append(spans, child.Span())
		default:
			ref := a.lowerExpr(child)
			if ref != expr.NoRef {
				items = append(items, ref)
				spans = append(spans, child.Span())
			}
		}
	}
	switch len(items) {
	case 0:
		return a.b.Const(n.Span(), value.None{})
	case 1:
		return items[0]
	default:
		return a.b.CodeJoin(n.Span(), items, spans)
	}
}

func (a *analyzer) lowerCodeInto(n syntax.Node, items []expr.Ref, spans []syntax.Span) ([]expr.Ref, []syntax.Span) {
	ns := a.inner(n, syntax.KindCode)
	for child := range ns.all() {
		switch child.Kind() {
		case syntax.KindSemicolon:
			continue
		case syntax.KindError:
			items = append(items, a.emitSyntaxError(child.(*syntax.Error)))
			spans = append(spans, child.Span())
		default:
			ref := a.lowerExpr(child)
			if ref != expr.NoRef {
				items = append(items, ref)
				spans = append(spans, child.Span())
			}
		}
	}
	return items, spans
}

// Conditionals ///////////////////////////////////////////////////////////////

// lowerConditional lowers `if cond { ... } else if cond { ... } else { ... }`
// into a chain of basic blocks. Each arm writes a synthetic result
// variable; the join block reads it back, which inserts a phi automatically
// via the Builder's Braun construction.
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
		thenResult := a.lowerBlock(bodyNode)
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
		elseResult := a.lowerBlock(elseBodyNode)
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

// lowerWhileLoop lowers `while cond { body }`. The loop accumulates each
// iteration's body value into a sentinel accumulator that is finalized
// (via [expr.Builder.LoopAccResult]) at the exit block to give the loop
// expression a value.
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

	accName := a.b.NewVar(name.Make("$acc"))
	accInit := a.b.LoopAccBegin(n.Span())
	a.b.WriteVar(accName, a.b.CurrentBlock(), accInit)

	header := a.b.NewBlock()
	body := a.b.NewBlock()
	exit := a.b.NewBlock()

	a.b.Jump(n.Span(), header)
	a.b.SetBlock(header)
	cond := a.lowerExpr(condNode)
	a.b.Branch(n.Span(), cond, body, exit)

	a.b.SetBlock(body)
	a.b.SealBlock(body)
	a.pushLoop(header, exit)
	bodyRef := a.lowerBlock(bodyNode)
	a.popLoop()
	prevAcc := a.b.ReadVar(accName, a.b.CurrentBlock())
	if bodyRef == expr.NoRef {
		bodyRef = a.b.Const(n.Span(), value.None{})
	}
	newAcc := a.b.LoopAccAdd(n.Span(), prevAcc, bodyRef)
	a.b.WriteVar(accName, a.b.CurrentBlock(), newAcc)
	a.b.Jump(n.Span(), header)

	a.b.SealBlock(header)
	a.b.SetBlock(exit)
	a.b.SealBlock(exit)
	finalAcc := a.b.ReadVar(accName, exit)
	return a.b.LoopAccResult(n.Span(), finalAcc)
}

// lowerForLoop lowers `for pattern in iterable { body }` with accumulator
// semantics matching [lowerWhileLoop].
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
	// point at the iterable, not the whole `for` statement.
	iter := a.b.IterOpen(iterableNode.Span(), iterable)

	accName := a.b.NewVar(name.Make("$acc"))
	accInit := a.b.LoopAccBegin(n.Span())
	a.b.WriteVar(accName, a.b.CurrentBlock(), accInit)

	header := a.b.NewBlock()
	body := a.b.NewBlock()
	exit := a.b.NewBlock()

	a.b.Jump(n.Span(), header)
	a.b.SetBlock(header)
	hasNext := a.b.IterHasNext(n.Span(), iter)
	a.b.Branch(n.Span(), hasNext, body, exit)

	a.b.SetBlock(body)
	a.b.SealBlock(body)
	a.openScope()
	elem := a.b.IterAdvance(n.Span(), iter)
	a.lowerDestructPattern(patternNode, elem)
	a.pushLoop(header, exit)
	bodyRef := a.lowerBlock(bodyNode)
	a.popLoop()
	if bodyRef == expr.NoRef {
		bodyRef = a.b.Const(n.Span(), value.None{})
	}
	prevAcc := a.b.ReadVar(accName, a.b.CurrentBlock())
	newAcc := a.b.LoopAccAdd(n.Span(), prevAcc, bodyRef)
	a.b.WriteVar(accName, a.b.CurrentBlock(), newAcc)
	a.closeScope()
	a.b.Jump(n.Span(), header)

	a.b.SealBlock(header)
	a.b.SetBlock(exit)
	a.b.SealBlock(exit)
	finalAcc := a.b.ReadVar(accName, exit)
	return a.b.LoopAccResult(n.Span(), finalAcc)
}

// lowerLoopBreak emits a Jump to the innermost loop's exit block.
func (a *analyzer) lowerLoopBreak(n syntax.Node) expr.Ref {
	l, ok := a.currentLoop()
	if !ok {
		return a.emitError(n.Span(), "break outside of loop")
	}
	a.b.Jump(n.Span(), l.exit)
	// Switch to a fresh unreachable block so subsequent emission has somewhere
	// to land, even though it'll be dead code.
	dead := a.b.NewBlock()
	a.b.SetBlock(dead)
	a.b.SealBlock(dead)
	return expr.NoRef
}

// lowerLoopContinue emits a Jump to the innermost loop's header.
func (a *analyzer) lowerLoopContinue(n syntax.Node) expr.Ref {
	l, ok := a.currentLoop()
	if !ok {
		return a.emitError(n.Span(), "continue outside of loop")
	}
	a.b.Jump(n.Span(), l.header)
	dead := a.b.NewBlock()
	a.b.SetBlock(dead)
	a.b.SealBlock(dead)
	return expr.NoRef
}

// lowerFuncReturn emits a Return terminator with the optional value. The
// resulting block is dead; subsequent emission goes into a fresh block.
func (a *analyzer) lowerFuncReturn(n syntax.Node) expr.Ref {
	ns := a.inner(n, syntax.KindFuncReturn)
	ns.take(syntax.KindReturn)
	val := expr.NoRef
	if !ns.done() {
		val = a.lowerExpr(ns.node())
	}
	a.b.Return(n.Span(), val)
	dead := a.b.NewBlock()
	a.b.SetBlock(dead)
	a.b.SealBlock(dead)
	return expr.NoRef
}

// Closures ///////////////////////////////////////////////////////////////////

// lowerClosure lowers a closure literal to a MakeClosure instruction. The
// closure body is constructed in a nested [expr.Function]; captures are
// allocated lazily as the body's lowerIdent calls encounter outer-scope
// names, then passed to MakeClosure once the body is fully lowered.
func (a *analyzer) lowerClosure(n syntax.Node) expr.Ref {
	return a.lowerClosureNamed(n, name.Invalid)
}

// lowerClosureNamed lowers a closure literal. recName, when non-invalid, is
// the let-binding name wrapping this closure (e.g. `f` in `let f = ...` or
// `let f(x) = ...`). It is bound inside the closure body to a [expr.DefSelf]
// reference, so direct recursion needs no capture, and is used as the
// function's display name.
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
			closureName = name.Make(n0.Text())
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
		a.expected(ns, "expression")
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
	a.b.Function().Span = n.Span()
	if closureName != name.Invalid {
		a.b.Function().Name = closureName.String()
	}

	// Bind the recursion name (if any) as a self-binding. The actual
	// [expr.DefSelf] is allocated lazily by [resolveName] only if the body
	// references the name, so non-recursive functions stay capture-free and
	// preserve their existing SSA shape.
	if recName != name.Invalid {
		if a.scope.bindings == nil {
			a.scope.bindings = make(map[name.Name]binding)
		}
		a.scope.bindings[recName] = binding{self: true}
	}

	// Register parameters, capturing outer-scope default values. Defaults
	// are added as captures first, so the [Function.Captures] slice has
	// defaults at the front (in declaration order) and body-captured names
	// afterwards. [MakeClosure]'s capture list mirrors this order.
	var defaultOuterRefs []expr.Ref // outer Refs to thread into MakeClosure
	for i, p := range paramSpecs {
		innerDefault := expr.NoRef
		if p.kind == expr.ParamNamed && defaultRefs[i] != expr.NoRef {
			// Display the capture as e.g. `default(b)` so the SSA dump shows
			// which param the captured default belongs to. Param names are
			// unique within a function, so Version 0 is fine.
			capVar := expr.Var{Name: name.Make("$default$" + p.name.String())}
			innerDefault = a.b.AddCapture(capVar, p.span)
			defaultOuterRefs = append(defaultOuterRefs, defaultRefs[i])
		}
		switch p.kind {
		case expr.ParamPositional:
			if p.name != name.Invalid {

				v := a.allocVar(p.name)
				paramRef := a.b.AddParam(p.name, expr.ParamPositional, expr.NoRef, p.span)
				a.b.WriteVar(v, expr.BlockID(0), paramRef)
			} else {
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

	// Lower the body.
	bodyRef := a.lowerBlockOrExpr(bodyNode)
	a.b.Return(n.Span(), bodyRef)

	// Capture the function and the capture-source list before swapping back.
	innerFn := a.b.Function()
	captures := innerFn.Captures
	a.popFrame()
	// Register the function in the module.
	mod := a.mb.Module()
	funcID := expr.FuncID(len(mod.Functions))
	mod.Functions = append(mod.Functions, innerFn)

	// Build the capture Ref list in outer scope. The first
	// len(defaultOuterRefs) entries are the default-value captures (in
	// declaration order); the remainder are body-referenced captures
	// resolved by source name.
	captureRefs := make([]expr.Ref, len(captures))
	copy(captureRefs, defaultOuterRefs)
	for i := len(defaultOuterRefs); i < len(captures); i++ {
		captureRefs[i] = a.resolveName(captures[i].Name, n.Span())
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
}

// collectClosureParamSpecs walks a Params (or single-ident) node and returns
// the param descriptors. Defaults are returned as syntax nodes to be lowered
// later by the caller (in whatever scope makes sense).
func (a *analyzer) collectClosureParamSpecs(n syntax.Node) []closureParamSpec {
	var out []closureParamSpec
	seen := make(map[name.Name]bool)
	addName := func(n name.Name, span syntax.Span) {
		if n == name.Invalid {
			return
		}
		if seen[n] {
			a.emitError(span, "duplicate parameter: "+n.String())
		}
		seen[n] = true
	}
	sawSink := false
	add := func(s closureParamSpec) {
		ns := s.nameSpan
		if ns == (syntax.Span{}) {
			ns = s.span
		}
		if s.kind == expr.ParamSink {
			if sawSink {
				a.emitError(s.span, "only one arguments sink is allowed")
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
func closureSourceName(n syntax.Node) (name.Name, bool) {
	inner, ok := n.(*syntax.Inner)
	if !ok {
		return name.Invalid, false
	}
	children := inner.Children()
	if len(children) >= 2 && children[0].Kind() == syntax.KindIdent && children[1].Kind() == syntax.KindParams {
		return name.Make(children[0].Text()), true
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
	ns.take(syntax.KindStar)
	body := a.lowerMarkup(ns.node())
	ns.take(syntax.KindStar)
	return a.b.Strong(n.Span(), body)
}

func (a *analyzer) lowerEmph(n syntax.Node) expr.Ref {
	ns := a.inner(n, syntax.KindEmph)
	ns.take(syntax.KindUnderscore)
	body := a.lowerMarkup(ns.node())
	ns.take(syntax.KindUnderscore)
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
	ns.take(syntax.KindLeftBracket)
	bodyNode := ns.node()
	ns.take(syntax.KindRightBracket)
	// A content block always evaluates to Content, even with a single item
	// (so e.g. `[#str]` yields content, not the raw string). Force a
	// ContentResult so the type-coercion happens at eval time.
	return a.lowerMarkupAsContent(bodyNode)
}

// lowerMarkupAsContent lowers a markup body to a single Ref that is always
// of content type at runtime — wrapping single items in a unary
// ContentResult so e.g. `[#42]` evaluates to content (with the int coerced
// to text) rather than the bare int.
func (a *analyzer) lowerMarkupAsContent(n syntax.Node) expr.Ref {
	items := a.lowerMarkupItems(n)
	return a.b.ContentResult(n.Span(), items)
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
	var lines []string
	for child := range ns.all() {
		switch child.Kind() {
		case syntax.KindRawDelim:
		case syntax.KindRawTrimmed:
			continue
		default:
			if child.Kind() != syntax.KindText {
				panic("invalid node kind in raw: " + child.Kind().String())
			}
			lines = append(lines, child.Text())
		}
	}
	return a.b.Const(n.Span(), &value.Raw{Block: marker != "`", Lang: lang, Lines: lines})
}

// Set/show/contextual/include stubs //////////////////////////////////////////
//
// These constructs have placeholder instructions in the IR; the evaluator
// panics on them today and will continue to do so until they're implemented
// in their own design pass.

func (a *analyzer) lowerSetRule(n syntax.Node) expr.Ref {
	ns := a.inner(n, syntax.KindSetRule)
	ns.take(syntax.KindSet)
	target := a.lowerExpr(ns.node())
	args, _ := a.lowerArgs(ns.node())
	cond := expr.NoRef
	if ns.at(syntax.KindIf) {
		ns.node()
		cond = a.lowerExpr(ns.node())
	}
	return a.b.SetRule(n.Span(), target, args, cond)
}

func (a *analyzer) lowerShowRule(n syntax.Node) expr.Ref {
	ns := a.inner(n, syntax.KindShowRule)
	ns.take(syntax.KindShow)
	selector := expr.NoRef
	if !ns.at(syntax.KindColon) {
		selector = a.lowerExpr(ns.node())
	}
	ns.take(syntax.KindColon)
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
	return a.b.Const(n.Span(), value.None{})
}

// assignDestructPattern is the assignment counterpart of
// [lowerDestructPattern]: each pattern leaf is an existing binding that
// the runtime should overwrite, not a fresh binding to allocate.
func (a *analyzer) assignDestructPattern(n syntax.Node, rhs expr.Ref) {
	switch n.Kind() {
	case syntax.KindIdent:
		source := name.Make(a.leaf(n, syntax.KindIdent))
		binding, ok := a.lookup(source)
		if !ok {
			a.checkIdent(source, n.Span())
			return
		}
		a.b.WriteVar(binding.variable, a.b.CurrentBlock(), rhs)
		return
	case syntax.KindUnderscore:
		return
	}

	ns := a.inner(n, syntax.KindDestructuring)
	type binding struct {
		ssaVar expr.Var
		span   syntax.Span
		idx    int
	}
	var bindings []binding
	idx := 0
	holes := 0
	for child := range ns.inside(syntax.KindLeftParen, syntax.KindRightParen) {
		switch child.Kind() {
		case syntax.KindComma:
			continue
		case syntax.KindError:
			a.emitSyntaxError(child.(*syntax.Error))
		case syntax.KindUnderscore:
			holes++
			idx++
		case syntax.KindIdent:
			source := name.Make(a.leaf(child, syntax.KindIdent))
			b, ok := a.lookup(source)
			if !ok {
				a.checkIdent(source, child.Span())
				idx++
				continue
			}
			bindings = append(bindings, binding{ssaVar: b.variable, span: child.Span(), idx: idx})
			idx++
		case syntax.KindNamed, syntax.KindSpread:
			// Named and sink patterns are accepted at analyze time but not
			// yet runtime-supported; the SSA simply skips them.
		}
	}
	if holes == 0 && len(bindings) == 1 && idx == 1 {
		a.b.WriteVar(bindings[0].ssaVar, a.b.CurrentBlock(), rhs)
		return
	}
	a.b.LengthCheck(n.Span(), rhs, idx, false)
	for _, b := range bindings {
		ref := a.b.Extract(b.span, rhs, b.idx)
		a.b.WriteVar(b.ssaVar, a.b.CurrentBlock(), ref)
	}
}

func (a *analyzer) lowerModuleInclude(n syntax.Node) expr.Ref {
	ns := a.inner(n, syntax.KindModuleInclude)
	ns.take(syntax.KindInclude)
	source := a.lowerExpr(ns.node())
	return a.b.ModuleInclude(n.Span(), source)
}
