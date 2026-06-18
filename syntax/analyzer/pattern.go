package analyzer

import (
	"znkr.io/writst/expr"
	"znkr.io/writst/name"
	"znkr.io/writst/syntax"
	"znkr.io/writst/value"
)

// This file holds the destructuring-pattern compiler shared by binding patterns
// (`let (a, b) = ...`, `for (a, b) in ...`, closure-param destructuring) and
// assignment patterns (`(a, b) = ...`). The two differ only at the leaves —
// fresh binding vs. lvalue write — and that split is carried by
// [patternCtx.assign].

// patternItem is one child of a destructuring pattern (`(a, b: c, ..d)`),
// classified by [patternCtx.compileDestructure] for compileArray / compileDict
// to consume.
type patternItem struct {
	kind syntax.Kind // KindIdent, KindUnderscore, KindParenthesized, KindDestructuring, KindNamed, KindSpread
	node syntax.Node // the syntax node itself
	// For KindNamed:
	key     name.Name
	keySpan syntax.Span
	sub     syntax.Node // the value-side pattern
	// For KindSpread:
	sinkTarget syntax.Node // ident/underscore inside the spread, or nil
}

// patternCtx is the shared state for compiling a destructuring pattern —
// either a binding pattern or an assignment pattern. The two differ at
// the leaves: binding allocates a fresh SSA variable and writes the
// value; assignment looks up an existing binding (or an arbitrary lvalue
// expression) and writes through it.
type patternCtx struct {
	a *analyzer
	// assign reports whether leaves are assignment lvalues (true) or
	// fresh bindings (false).
	assign bool
	// seen tracks duplicate-binding diagnostics within a single pattern
	// (binding-mode only).
	seen map[name.Name]struct{}
}

// compile is the recursive entry point. It dispatches by syntactic kind to
// either a leaf bind/assign or a sub-pattern walk.
func (pc *patternCtx) compile(n syntax.Node, rhs expr.Ref) {
	switch n.Kind() {
	case syntax.KindIdent:
		pc.bindLeaf(n, rhs)
		return
	case syntax.KindUnderscore:
		return
	case syntax.KindError:
		pc.a.emitSyntaxError(n.(*syntax.Error))
		return
	case syntax.KindParenthesized:
		// Grouping: unwrap and recurse on the single inner expression.
		ns := pc.a.inner(n, syntax.KindParenthesized)
		for child := range ns.inside(syntax.KindLeftParen, syntax.KindRightParen) {
			pc.compile(child, rhs)
		}
		return
	case syntax.KindDestructuring:
		pc.compileDestructure(n, rhs)
		return
	}
	if pc.assign {
		// Assignment-mode leaves can be any lvalue (FieldAccess, FuncCall,
		// nested parens-of-Call). Delegate to the shared lvalue writer.
		pc.a.writeLValue(n.Span(), n, syntax.Assign, rhs)
		return
	}
	pc.a.emitError(n.Span(), "expected pattern, found "+n.Kind().Name())
}

// bindLeaf writes rhs to an identifier leaf — either creating a fresh
// binding (binding mode) or updating an existing one (assignment mode).
func (pc *patternCtx) bindLeaf(n syntax.Node, rhs expr.Ref) {
	a := pc.a
	source := name.Make(a.leaf(n, syntax.KindIdent))
	if pc.assign {
		bnd, ok := a.lookup(source)
		if !ok {
			a.checkIdent(source, n.Span())
			return
		}
		a.b.WriteVar(bnd.(varBinding).v, a.b.CurrentBlock(), rhs)
		return
	}
	if _, dup := pc.seen[source]; dup {
		a.emitError(n.Span(), "duplicate binding: "+source.String())
		return
	}
	pc.seen[source] = struct{}{}
	v := a.allocVar(source)
	a.b.WriteVar(v, a.b.CurrentBlock(), rhs)
}

// compileDestructure handles a KindDestructuring node. It first walks the
// children to classify the pattern (array vs dict), validate
// duplicates/sinks, and collect a normalised item list, then emits the
// right IR.
func (pc *patternCtx) compileDestructure(n syntax.Node, rhs expr.Ref) {
	a := pc.a
	ns := a.inner(n, syntax.KindDestructuring)

	var items []patternItem
	var firstNamedSpan syntax.Span
	hasNamed := false
	hasSink := false
	sinkAt := -1
	hadError := false

	for child := range ns.inside(syntax.KindLeftParen, syntax.KindRightParen) {
		switch child.Kind() {
		case syntax.KindComma:
			continue
		case syntax.KindError:
			hadError = true
			a.emitSyntaxError(child.(*syntax.Error))
		case syntax.KindSpread:
			if hasSink {
				a.emitError(child.Span(), "only one destructuring sink is allowed")
				continue
			}
			hasSink = true
			sinkAt = len(items)
			spread := a.inner(child, syntax.KindSpread)
			spread.take(syntax.KindDots)
			it := patternItem{kind: syntax.KindSpread, node: child}
			if !spread.done() {
				inner := spread.node()
				switch inner.Kind() {
				case syntax.KindIdent, syntax.KindUnderscore:
					it.sinkTarget = inner
				case syntax.KindError:
					hadError = true
					a.emitSyntaxError(inner.(*syntax.Error))
				default:
					if pc.assign {
						it.sinkTarget = inner
					} else {
						hadError = true
						a.emitError(inner.Span(), "expected pattern, found "+inner.Kind().Name())
					}
				}
			}
			items = append(items, it)
		case syntax.KindNamed:
			named := a.inner(child, syntax.KindNamed)
			keyNode := named.node()
			if keyNode.Kind() == syntax.KindError {
				hadError = true
				a.emitSyntaxError(keyNode.(*syntax.Error))
				// Bind any ident on the value side to none so later
				// references don't cascade into "unknown variable"
				// diagnostics on top of the underlying pattern error.
				named.take(syntax.KindColon)
				patNode := named.node()
				switch patNode.Kind() {
				case syntax.KindError:
					a.emitSyntaxError(patNode.(*syntax.Error))
				case syntax.KindIdent:
					if !pc.assign {
						target := name.Make(a.leaf(patNode, syntax.KindIdent))
						if _, dup := pc.seen[target]; !dup {
							pc.seen[target] = struct{}{}
							v := a.allocVar(target)
							none := a.b.Const(patNode.Span(), value.None{})
							a.b.WriteVar(v, a.b.CurrentBlock(), none)
						}
					}
				}
				continue
			}
			if keyNode.Kind() != syntax.KindIdent {
				hadError = true
				a.emitError(keyNode.Span(), "expected identifier, found "+keyNode.Kind().Name())
				continue
			}
			keyText := a.leaf(keyNode, syntax.KindIdent)
			key := name.Make(keyText)
			named.take(syntax.KindColon)
			patNode := named.node()
			if patNode.Kind() == syntax.KindError {
				hadError = true
				a.emitSyntaxError(patNode.(*syntax.Error))
				continue
			}
			if !hasNamed {
				firstNamedSpan = child.Span()
			}
			hasNamed = true
			items = append(items, patternItem{kind: syntax.KindNamed, node: child, key: key, keySpan: keyNode.Span(), sub: patNode})
		default:
			if a.collectChildErrors(child, true) {
				hadError = true
				continue
			}
			items = append(items, patternItem{kind: child.Kind(), node: child})
		}
	}

	if hadError {
		return
	}

	if hasNamed {
		pc.compileDictDestructure(n, rhs, items, hasSink, firstNamedSpan)
		return
	}
	pc.compileArrayDestructure(n, rhs, items, hasSink, sinkAt)
}

// patternIsHybrid reports whether a positional pattern can also
// destructure a dict via ident shorthand: every item must be a plain
// ident, an underscore-less spread, or a spread carrying an ident
// target. Nested patterns, holes, and lvalue expressions disable the
// hybrid path (those have no key name to look up).
func patternIsHybrid(items []patternItem) bool {
	for _, it := range items {
		switch it.kind {
		case syntax.KindIdent:
			continue
		case syntax.KindSpread:
			// Sink slot has no key to look up; allowed regardless of
			// whether it has a target.
			if it.sinkTarget == nil {
				continue
			}
			if it.sinkTarget.Kind() == syntax.KindIdent || it.sinkTarget.Kind() == syntax.KindUnderscore {
				continue
			}
			return false
		default:
			return false
		}
	}
	return true
}

// compileArrayDestructure emits IR for a positional destructuring pattern
// (with optional sink). When the pattern is pure-ident-positional, the
// runtime also accepts a dict source (shorthand semantics).
func (pc *patternCtx) compileArrayDestructure(n syntax.Node, rhs expr.Ref, items []patternItem, hasSink bool, sinkAt int) {
	a := pc.a
	before, after := 0, 0
	if hasSink {
		before = sinkAt
		after = len(items) - sinkAt - 1
	} else {
		before = len(items)
	}
	hybrid := patternIsHybrid(items)
	var keys []name.Name
	if hybrid {
		keys = make([]name.Name, len(items))
		for i, it := range items {
			if it.kind == syntax.KindIdent {
				keys[i] = name.Make(a.leaf(it.node, syntax.KindIdent))
			}
		}
	}
	validated := a.b.DestructArray(n.Span(), rhs, before, after, hasSink, hybrid)
	for i, it := range items {
		var key name.Name
		if hybrid && it.kind == syntax.KindIdent {
			key = keys[i]
		}
		switch {
		case hasSink && i == sinkAt:
			if it.sinkTarget == nil {
				continue
			}
			slice := a.b.ArraySlice(it.node.Span(), validated, before, after, hybrid, keys)
			pc.compile(it.sinkTarget, slice)
		case hasSink && i > sinkAt:
			fromEnd := len(items) - 1 - i
			elem := a.b.ArrayElem(it.node.Span(), validated, fromEnd, true, key)
			pc.compile(it.node, elem)
		default:
			elem := a.b.ArrayElem(it.node.Span(), validated, i, false, key)
			pc.compile(it.node, elem)
		}
	}
}

// compileDictDestructure emits IR for a dict-shape destructuring pattern
// (any pattern with at least one `key: value` pair). A positional ident
// inside a dict pattern means shorthand `name: name`.
func (pc *patternCtx) compileDictDestructure(n syntax.Node, rhs expr.Ref, items []patternItem, hasSink bool, firstNamedSpan syntax.Span) {
	a := pc.a
	// Collect consumed keys (for sink filtering and dict validation).
	var consumed []name.Name
	for _, it := range items {
		switch it.kind {
		case syntax.KindIdent:
			consumed = append(consumed, name.Make(a.leaf(it.node, syntax.KindIdent)))
		case syntax.KindNamed:
			consumed = append(consumed, it.key)
		}
	}
	validated := a.b.DestructDict(n.Span(), firstNamedSpan, rhs, consumed, hasSink)
	for _, it := range items {
		switch it.kind {
		case syntax.KindIdent:
			key := name.Make(a.leaf(it.node, syntax.KindIdent))
			field := a.b.DictField(it.node.Span(), it.node.Span(), validated, key)
			pc.bindLeaf(it.node, field)
		case syntax.KindUnderscore:
			// Stray underscore in a dict pattern — silently skip.
		case syntax.KindNamed:
			field := a.b.DictField(it.sub.Span(), it.keySpan, validated, it.key)
			pc.compile(it.sub, field)
		case syntax.KindSpread:
			if it.sinkTarget == nil {
				continue
			}
			rest := a.b.DictRest(it.node.Span(), validated, consumed)
			pc.compile(it.sinkTarget, rest)
		}
	}
}
