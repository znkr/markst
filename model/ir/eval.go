package ir

import (
	"fmt"
	"unique"

	"znkr.io/writst/syntax"
)

func Eval(ec *EvalContext, exprs []Expr) (Contents, error) {
	ec.openScope()
	defer ec.closeScope()
	return evalContents(ec, exprs), nil
}

// Context /////////////////////////////////////////////////////////////////////////////////////////

type EvalContext struct {
	scope *scope
}

func NewEvalContext() *EvalContext {
	return &EvalContext{
		scope: &scope{
			parent: universe,
		},
	}
}

func (ec *EvalContext) openScope() {
	ec.scope = &scope{
		parent: ec.scope,
	}
}

func (ec *EvalContext) closeScope() {
	ec.scope = ec.scope.parent
}

func (ec *EvalContext) Lookup(name unique.Handle[string]) (Value, bool) {
	return ec.scope.lookup(name)
}

func (ec *EvalContext) Bind(name unique.Handle[string], val Value) {
	if ec.scope.bindings == nil {
		ec.scope.bindings = make(map[unique.Handle[string]]Value)
	}
	ec.scope.bindings[name] = val
}

// Scope ///////////////////////////////////////////////////////////////////////////////////////////

var universe = &scope{
	bindings: builtins,
}

type scope struct {
	parent   *scope
	bindings map[unique.Handle[string]]Value
}

func (s *scope) lookup(name unique.Handle[string]) (Value, bool) {
	if val, ok := s.bindings[name]; ok {
		return val, true
	}
	if s.parent != nil {
		return s.parent.lookup(name)
	}
	return nil, false
}

// Content Expressions /////////////////////////////////////////////////////////////////////////////

func (n *HeadingExpr) eval(ec *EvalContext) Value {
	return &Heading{
		Level: n.level,
		Body:  evalContents(ec, n.body),
	}
}

func (n *StrongExpr) eval(ec *EvalContext) Value {
	return &Strong{
		Body: evalContents(ec, n.body),
	}
}

func (n *EmphExpr) eval(ec *EvalContext) Value {
	return &Emph{
		Body: evalContents(ec, n.body),
	}
}

func (n *LinkExpr) eval(ec *EvalContext) Value {
	return &Link{
		Dest: n.dest,
		Body: evalContents(ec, n.body),
	}
}

func (n *RefExpr) eval(ec *EvalContext) Value {
	return &Ref{
		Target:     n.target,
		Supplement: n.supplement.eval(ec).(Content),
	}
}

func (n *ListItemExpr) eval(ec *EvalContext) Value {
	return &ListItem{
		Body: evalContents(ec, n.body),
	}
}

func (n *EnumItemExpr) eval(ec *EvalContext) Value {
	return &EnumItem{
		Number: n.number,
		Body:   evalContents(ec, n.body),
	}
}

func (n *TermItemExpr) eval(ec *EvalContext) Value {
	return &TermItem{
		Term:        evalContents(ec, n.term),
		Description: evalContents(ec, n.description),
	}
}

func evalContents(ec *EvalContext, exprs []Expr) Contents {
	var ret Contents
	for _, expr := range exprs {
		v := toContent(expr.eval(ec))
		if v == nil {
			continue
		}
		ret = append(ret, v)
	}
	return ret
}

func toContent(v Value) Content {
	switch v := v.(type) {
	case Content:
		return v
	case String:
		return &Text{Value: string(v)}
	case None:
		return nil
	default:
		panic(fmt.Sprintf("content expression evaluated to non-element value: %T", v))
	}
}

// Code ////////////////////////////////////////////////////////////////////////////////////////////

func (n *Const) eval(ec *EvalContext) Value { return n.value }

// Code Expressions ////////////////////////////////////////////////////////////////////////////////

func (n *Ident) eval(ec *EvalContext) Value {
	val, ok := ec.Lookup(n.name)
	if !ok {
		panic("undefined identifier: " + n.name.Value())
	}
	return val
}

func (n *CodeBlock) eval(ec *EvalContext) Value {
	ec.openScope()
	defer ec.closeScope()
	var array Array
	for _, expr := range n.exprs {
		array = append(array, expr.eval(ec))
	}
	return array
}

func (n *ContentBlock) eval(ec *EvalContext) Value {
	ec.openScope()
	defer ec.closeScope()
	return evalContents(ec, n.exprs)
}

func (n *Parenthesized) eval(ec *EvalContext) Value {
	return n.body.eval(ec)
}

// Collections /////////////////////////////////////////////////////////////////////////////////////

func (n *ArrayExpr) eval(ec *EvalContext) Value {
	elems := make(Array, 0, len(n.elements))
	for _, expr := range n.elements {
		elems = append(elems, expr.eval(ec))
	}
	return elems
}

func (n *DictExpr) eval(ec *EvalContext) Value {
	dict := make(Dict, len(n.entries))
	for _, ent := range n.entries {
		keyVal := ent.key.eval(ec)
		keyStr, ok := keyVal.(String)
		if !ok {
			panic("dictionary key did not evaluate to a string")
		}
		dict[keyStr] = ent.value.eval(ec)
	}
	return dict
}

// Operations //////////////////////////////////////////////////////////////////////////////////////

type unaryopKey struct {
	op          syntax.UnaryOp
	operandKind Kind
}

var unaryops = map[unaryopKey]func(x Value) Value{
	{syntax.Not, KindBool}: func(x Value) Value {
		return !x.(Bool)
	},
	{syntax.Neg, KindInt}: func(x Value) Value {
		return -x.(Int)
	},
	{syntax.Neg, KindFloat}: func(x Value) Value {
		return -x.(Float)
	},
}

type binopKey struct {
	op        syntax.BinaryOp
	leftKind  Kind
	rightKind Kind
}

var binops = map[binopKey]func(x, y Value) Value{
	// Int operations
	{syntax.Add, KindInt, KindInt}: func(x, y Value) Value {
		return x.(Int) + y.(Int)
	},
	{syntax.Sub, KindInt, KindInt}: func(x, y Value) Value {
		return x.(Int) - y.(Int)
	},
	{syntax.Mul, KindInt, KindInt}: func(x, y Value) Value {
		return x.(Int) * y.(Int)
	},
	{syntax.Div, KindInt, KindInt}: func(x, y Value) Value {
		return x.(Int) / y.(Int)
	},

	// Float operations
	{syntax.Add, KindFloat, KindFloat}: func(x, y Value) Value {
		return x.(Float) + y.(Float)
	},
	{syntax.Sub, KindFloat, KindFloat}: func(x, y Value) Value {
		return x.(Float) - y.(Float)
	},
	{syntax.Mul, KindFloat, KindFloat}: func(x, y Value) Value {
		return x.(Float) * y.(Float)
	},
	{syntax.Div, KindFloat, KindFloat}: func(x, y Value) Value {
		return x.(Float) / y.(Float)
	},

	{syntax.Add, KindString, KindString}: func(x, y Value) Value {
		return String(string(x.(String)) + string(y.(String)))
	},
}

func (n *Unary) eval(ec *EvalContext) Value {
	x := n.operand.eval(ec)
	op := unaryops[unaryopKey{n.op, x.Kind()}]
	if op == nil {
		panic(fmt.Sprintf("unsupported unary operation: %s %s", n.op, x.Kind()))
	}
	return op(x)
}

func (n *Binary) eval(ec *EvalContext) Value {
	left, right := n.left.eval(ec), n.right.eval(ec)
	op := binops[binopKey{n.op, left.Kind(), right.Kind()}]
	if op == nil {
		panic(fmt.Sprintf("unsupported binary operation: %s %s %s", left.Kind(), n.op, right.Kind()))
	}
	return op(left, right)
}

func (n *FieldAccess) eval(ec *EvalContext) Value {
	panic("TODO: implement field access")
}

// Functions ///////////////////////////////////////////////////////////////////////////////////////

func (n *FuncCall) eval(ec *EvalContext) Value {
	callee := n.callee.eval(ec)
	fn, ok := callee.(*Function)
	if !ok {
		panic("attempted to call a non-function value")
	}

	var args Arguments
	for _, arg := range n.args {
		switch a := arg.(type) {
		case *ExprArg:
			args.Positional = append(args.Positional, a.expr.eval(ec))
		case *NamedArg:
			if args.Named == nil {
				args.Named = make(map[unique.Handle[string]]Value)
			}
			args.Named[a.name] = a.expr.eval(ec)
		case *SpreadArg:
			panic("TODO: implement spread arguments")
		default:
			panic("unrecognized function argument type")
		}
	}
	return fn.Apply(&args)
}

func (n *Closure) eval(ec *EvalContext) Value {
	panic("TODO: not actually an expression")
}

// Bindings & Rules ////////////////////////////////////////////////////////////////////////////////

func (n *LetBinding) eval(ec *EvalContext) Value {
	for _, p := range n.pattern {
		switch p := p.(type) {
		case *DestructIdent:
			ec.Bind(p.ident.name, n.value.eval(ec))
		default:
			panic("TODO: implement complex let patterns")
		}
	}
	return &None{}
}

func (n *DestructAssignment) eval(ec *EvalContext) Value {
	panic("TODO: implement destruct assignment")
}

func (n *SetRule) eval(ec *EvalContext) Value {
	panic("TODO: implement set rules")
}

func (n *ShowRule) eval(ec *EvalContext) Value {
	panic("TODO: implement show rules")
}

// Control Flow ////////////////////////////////////////////////////////////////////////////////////

func (n *Conditional) eval(ec *EvalContext) Value {
	for i, cond := range n.conditions {
		cond, ok := cond.eval(ec).(Bool)
		if !ok {
			panic("condition did not evaluate to a boolean")
		}
		if cond {
			return n.blocks[i].eval(ec)
		}
	}
	if n.def != nil {
		return n.def.eval(ec)
	}
	return &None{}
}

func (n *ForLoop) eval(ec *EvalContext) Value {
	panic("TODO: implement for loops")
}

func (n *WhileLoop) eval(ec *EvalContext) Value {
	panic("TODO: implement while loops")
}

func (n *LoopBreak) eval(ec *EvalContext) Value {
	panic("TODO: not actually an expression")
}

func (n *LoopContinue) eval(ec *EvalContext) Value {
	panic("TODO: not actually an expression")
}

func (n *FuncReturn) eval(ec *EvalContext) Value {
	panic("TODO: not actually an expression")
}

// Other ///////////////////////////////////////////////////////////////////////////////////////////

func (n *Contextual) eval(ec *EvalContext) Value { panic("TODO: implement contextual") }

func (n *ModuleInclude) eval(ec *EvalContext) Value { panic("TODO: implement module include") }
