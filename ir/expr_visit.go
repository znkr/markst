package ir

import "slices"

// VisitChildren calls fn(x) on each of n's non-nil child expressions x. If any call returns true,
// VisitChildren stops and returns true. Otherwise, VisitChildren returns false.
//
// Note that VisitChildren(n, fn) only calls fn(x) for n's immediate children. If x's children
// should be processed, then fn(x) must call VisitChildren(x, fn).
//
// VisitChildren allows constructing general traversals of the IR graph that can stop early if
// needed. The most general usage is:
//
//	var fn func(ir.Expr) bool
//	fn = func(x ir.Expr) bool {
//	    ... processing BEFORE visiting children ...
//	    if ... should visit children ... {
//	        ir.VisitChildren(x, fn)
//	        ... processing AFTER visiting children ...
//	    }
//	    if ... should stop parent VisitChildren call from visiting siblings ... {
//	        return true
//	    }
//	    return false
//	}
//	fn(root)
//
// Since VisitChildren does not return true itself, if the fn function never wants to stop the
// traversal, it can assume that VisitChildren itself will always return false, simplifying to:
//
//	var fn func(ir.Expr) bool
//	fn = func(x ir.Expr) bool {
//	    ... processing BEFORE visiting children ...
//	    if ... should visit children ... {
//	        ir.VisitChildren(x, fn)
//	    }
//	    ... processing AFTER visiting children ...
//	    return false
//	}
//	fn(root)
func VisitChildren(n Expr, fn func(Expr) bool) bool {
	return n.visitChildren(fn)
}

// VisitList visits each expression in the list.
func VisitList(ns []Expr, fn func(Expr) bool) bool {
	return slices.ContainsFunc(ns, fn)
}

// Visit visits each expression x in the IR tree rooted at n in a depth-first preorder traversal,
// calling visit on each node visited.
func Visit(n Expr, visit func(Expr)) {
	var fn func(Expr) bool
	fn = func(x Expr) bool {
		visit(x)
		return VisitChildren(x, fn)
	}
	fn(n)
}

// Any looks for an expression x in the IR tree rooted at n for which cond(x) returns true. Any
// considers nodes in a depth-first, preorder traversal.
//
// When Any finds a node x such that cond(x) is true, Any ends the traversal and returns true
// immediately. Otherwise Any returns false after completing the entire traversal.
func Any(n Expr, cond func(Expr) bool) bool {
	var fn func(Expr) bool
	fn = func(x Expr) bool {
		return cond(x) || VisitChildren(x, fn)
	}
	return fn(n)
}

// visitArgs visits the expressions contained in an argument list.
func visitArgs(args []Arg, fn func(Expr) bool) bool {
	for _, a := range args {
		switch a := a.(type) {
		case *ExprArg:
			if fn(a.expr) {
				return true
			}
		case *NamedArg:
			if fn(a.expr) {
				return true
			}
		case *SpreadArg:
			if fn(a.expr) {
				return true
			}
		}
	}
	return false
}

// visitClosureParams visits the expressions contained in closure parameters.
func visitClosureParams(params []ClosureParam, fn func(Expr) bool) bool {
	for _, p := range params {
		switch p := p.(type) {
		case *PositionalClosureParam:
			if fn(p.name) {
				return true
			}
		case *NamedClosureParam:
			if fn(p.name) {
				return true
			}
			if p.def != nil && fn(p.def) {
				return true
			}
		case *SpreadClosureParam:
			if fn(p.ident) {
				return true
			}
		}
	}
	return false
}

// visitDestructPatterns visits the expressions contained in destructuring patterns.
func visitDestructPatterns(patterns []DestructPattern, fn func(Expr) bool) bool {
	for _, p := range patterns {
		switch p := p.(type) {
		case *DestructIdent:
			if fn(p.ident) {
				return true
			}
		case *DestructNamed:
			if fn(p.pattern) {
				return true
			}
		case *DestructSink:
			if p.ident != nil && fn(p.ident) {
				return true
			}
		}
	}
	return false
}

// Content Expressions

func (n *HeadingExpr) visitChildren(fn func(Expr) bool) bool  { return VisitList(n.body, fn) }
func (n *StrongExpr) visitChildren(fn func(Expr) bool) bool   { return VisitList(n.body, fn) }
func (n *EmphExpr) visitChildren(fn func(Expr) bool) bool     { return VisitList(n.body, fn) }
func (n *LinkExpr) visitChildren(fn func(Expr) bool) bool     { return VisitList(n.body, fn) }
func (n *ListItemExpr) visitChildren(fn func(Expr) bool) bool { return VisitList(n.body, fn) }
func (n *EnumItemExpr) visitChildren(fn func(Expr) bool) bool { return VisitList(n.body, fn) }

func (n *RefExpr) visitChildren(fn func(Expr) bool) bool {
	return n.supplement != nil && fn(n.supplement)
}

func (n *TermItemExpr) visitChildren(fn func(Expr) bool) bool {
	return VisitList(n.term, fn) || VisitList(n.description, fn)
}

// Code Expressions

func (n *ConstExpr) visitChildren(fn func(Expr) bool) bool { return false }
func (n *Ident) visitChildren(fn func(Expr) bool) bool     { return false }

func (n *CodeBlock) visitChildren(fn func(Expr) bool) bool     { return VisitList(n.exprs, fn) }
func (n *ContentBlock) visitChildren(fn func(Expr) bool) bool  { return VisitList(n.exprs, fn) }
func (n *Parenthesized) visitChildren(fn func(Expr) bool) bool { return fn(n.body) }

// Collections

func (n *ArrayExpr) visitChildren(fn func(Expr) bool) bool  { return VisitList(n.elements, fn) }
func (n *SpreadExpr) visitChildren(fn func(Expr) bool) bool { return fn(n.inner) }

func (n *DictExpr) visitChildren(fn func(Expr) bool) bool {
	for _, e := range n.entries {
		if fn(e.key) || fn(e.value) {
			return true
		}
	}
	return false
}

// Operators

func (n *Unary) visitChildren(fn func(Expr) bool) bool  { return fn(n.operand) }
func (n *Binary) visitChildren(fn func(Expr) bool) bool { return fn(n.left) || fn(n.right) }

func (n *FieldAccess) visitChildren(fn func(Expr) bool) bool {
	return fn(n.target) || fn(n.field)
}

// Functions

func (n *FuncCall) visitChildren(fn func(Expr) bool) bool {
	if fn(n.callee) {
		return true
	}
	if visitArgs(n.args, fn) {
		return true
	}
	for _, b := range n.blocks {
		if fn(b) {
			return true
		}
	}
	return false
}

func (n *Closure) visitChildren(fn func(Expr) bool) bool {
	if n.name != nil && fn(n.name) {
		return true
	}
	if visitClosureParams(n.params, fn) {
		return true
	}
	return fn(n.body)
}

// Bindings & Rules

func (n *LetBinding) visitChildren(fn func(Expr) bool) bool {
	return visitDestructPatterns(n.pattern, fn) || fn(n.value)
}

func (n *SetRule) visitChildren(fn func(Expr) bool) bool {
	if fn(n.target) {
		return true
	}
	if visitArgs(n.args, fn) {
		return true
	}
	return n.condition != nil && fn(n.condition)
}

func (n *ShowRule) visitChildren(fn func(Expr) bool) bool {
	if n.selector != nil && fn(n.selector) {
		return true
	}
	return fn(n.transform)
}

// Destructuring

func (n *Destructuring) visitChildren(fn func(Expr) bool) bool { return VisitList(n.items, fn) }

func (n *DestructAssignment) visitChildren(fn func(Expr) bool) bool {
	return visitDestructPatterns(n.pattern, fn) || fn(n.value)
}

// Control Flow

func (n *Conditional) visitChildren(fn func(Expr) bool) bool {
	return VisitList(n.conditions, fn) || VisitList(n.blocks, fn) || (n.def != nil && fn(n.def))
}

func (n *WhileLoop) visitChildren(fn func(Expr) bool) bool {
	return fn(n.condition) || fn(n.body)
}

func (n *ForLoop) visitChildren(fn func(Expr) bool) bool {
	return visitDestructPatterns(n.pattern, fn) || fn(n.iterable) || fn(n.body)
}

func (n *LoopBreak) visitChildren(fn func(Expr) bool) bool    { return false }
func (n *LoopContinue) visitChildren(fn func(Expr) bool) bool { return false }

func (n *FuncReturn) visitChildren(fn func(Expr) bool) bool {
	return n.value != nil && fn(n.value)
}

// Other

func (n *Contextual) visitChildren(fn func(Expr) bool) bool    { return fn(n.body) }
func (n *ModuleInclude) visitChildren(fn func(Expr) bool) bool { return fn(n.source) }
