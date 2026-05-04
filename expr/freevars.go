package expr

import (
	"maps"
	"unique"

	"znkr.io/writst/syntax"
)

// FreeVars returns the set of variable names referenced by n that are not
// locally bound within n. The bound parameter provides names already known to
// be in scope (e.g. a closure's own parameters). The result is de-duplicated
// and returned in order of first occurrence, with the span of the first
// reference to each variable.
func FreeVars(n Expr, bound map[unique.Handle[string]]bool) []Capture {
	var free []Capture
	seen := make(map[unique.Handle[string]]bool)
	addFree := func(name unique.Handle[string], span syntax.Span) {
		if !seen[name] {
			seen[name] = true
			free = append(free, Capture{Name: name, Span: span})
		}
	}
	freeVars(n, bound, addFree)
	return free
}

func freeVars(n Expr, bound map[unique.Handle[string]]bool, addFree func(unique.Handle[string], syntax.Span)) {
	switch n := n.(type) {
	case *Ident:
		if !bound[n.name] {
			addFree(n.name, n.span)
		}

	case *CodeBlock:
		freeVarsSeq(n.exprs, bound, addFree)

	case *ContentBlock:
		freeVarsSeq(n.exprs, bound, addFree)

	case *LetBinding:
		// The value is evaluated before the binding takes effect,
		// so "let x = x + 1" captures the outer x.
		if n.value != nil {
			freeVars(n.value, bound, addFree)
		}
		// Don't recurse into pattern idents — they are binding sites, not references.

	case *ForLoop:
		freeVars(n.iterable, bound, addFree)
		inner := maps.Clone(bound)
		bindPatterns(inner, n.pattern)
		freeVars(n.body, inner, addFree)

	case *Closure:
		inner := maps.Clone(bound)
		if n.name != nil {
			inner[n.name.name] = true
		}
		for _, p := range n.params {
			switch p := p.(type) {
			case *PositionalClosureParam:
				inner[p.name.name] = true
			case *NamedClosureParam:
				inner[p.name.name] = true
				// Default values are evaluated in the outer scope.
				if p.def != nil {
					freeVars(p.def, bound, addFree)
				}
			case *SpreadClosureParam:
				inner[p.ident.name] = true
			}
		}
		freeVars(n.body, inner, addFree)

	case *FieldAccess:
		freeVars(n.Target(), bound, addFree)

	default:
		VisitChildren(n, func(child Expr) bool {
			freeVars(child, bound, addFree)
			return false
		})
	}
}

// freeVarsSeq handles a sequence of expressions where let bindings accumulate
// in scope for subsequent expressions (as in a code block).
func freeVarsSeq(exprs []Expr, bound map[unique.Handle[string]]bool, addFree func(unique.Handle[string], syntax.Span)) {
	inner := maps.Clone(bound)
	for _, e := range exprs {
		freeVars(e, inner, addFree)
		// After processing, add any names this expression binds.
		if let, ok := e.(*LetBinding); ok {
			bindPatterns(inner, let.pattern)
		}
	}
}

func bindPatterns(bound map[unique.Handle[string]]bool, patterns []DestructPattern) {
	for _, p := range patterns {
		switch p := p.(type) {
		case *DestructIdent:
			bound[p.ident.name] = true
		case *DestructNamed:
			bound[p.pattern.name] = true
		case *DestructSink:
			if p.ident != nil {
				bound[p.ident.name] = true
			}
		}
	}
}
