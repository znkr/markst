package ir

import (
	"fmt"
	"unique"

	"znkr.io/writst/syntax"
)

var universe = &scope{
	bindings: builtins,
}

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

func (ec *EvalContext) PushScope() {
	ec.scope = &scope{
		parent: ec.scope,
	}
}

func (ec *EvalContext) PopScope() {
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

func (n *ContentExpr) Eval(ec *EvalContext) Value {
	var ret Contents
	for _, expr := range n.exprs {
		v := toContent(expr.Eval(ec))
		if v == nil {
			continue
		}
		ret = append(ret, v)
	}
	return ret
}

func (n *HeadingExpr) Eval(ec *EvalContext) Value {
	return &Heading{
		Level: n.level,
		Body:  n.body.Eval(ec).(Content),
	}
}

func (n *StrongExpr) Eval(ec *EvalContext) Value {
	return &Strong{
		Body: n.body.Eval(ec).(Content),
	}
}

func (n *EmphExpr) Eval(ec *EvalContext) Value {
	return &Emph{
		Body: n.body.Eval(ec).(Content),
	}
}

func (n *LinkExpr) Eval(ec *EvalContext) Value {
	return &Link{
		Dest: n.dest,
		Body: n.body.Eval(ec).(Content),
	}
}

func (n *RefExpr) Eval(ec *EvalContext) Value {
	return &Ref{
		Target:     n.target,
		Supplement: n.supplement.Eval(ec).(Content),
	}
}

func (n *ListItemExpr) Eval(ec *EvalContext) Value {
	return &ListItem{
		Body: n.body.Eval(ec).(Content),
	}
}

func (n *EnumItemExpr) Eval(ec *EvalContext) Value {
	return &EnumItem{
		Number: n.number,
		Body:   n.body.Eval(ec).(Content),
	}
}

func (n *TermItemExpr) Eval(ec *EvalContext) Value {
	return &TermItem{
		Term:        n.term.Eval(ec).(Content),
		Description: n.description.Eval(ec).(Content),
	}
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

func (n *Const) Eval(ec *EvalContext) Value { return n.value }

// Code Expressions ////////////////////////////////////////////////////////////////////////////////

func (n *Ident) Eval(ec *EvalContext) Value {
	val, ok := ec.Lookup(n.name)
	if !ok {
		panic("undefined identifier: " + n.name.Value())
	}
	return val
}

func (n *CodeBlock) Eval(ec *EvalContext) Value {
	ec.PushScope()
	defer ec.PopScope()
	return n.body.Eval(ec)
}

func (n *ContentBlock) Eval(ec *EvalContext) Value {
	ec.PushScope()
	defer ec.PopScope()
	return n.body.Eval(ec)
}

func (n *Parenthesized) Eval(ec *EvalContext) Value {
	return n.body.Eval(ec)
}

// Collections /////////////////////////////////////////////////////////////////////////////////////

func (n *ArrayExpr) Eval(ec *EvalContext) Value {
	elems := make(Array, 0, len(n.elements))
	for _, expr := range n.elements {
		elems = append(elems, expr.Eval(ec))
	}
	return elems
}

func (n *DictExpr) Eval(ec *EvalContext) Value {
	dict := make(Dict, len(n.entries))
	for _, ent := range n.entries {
		keyVal := ent.key.Eval(ec)
		keyStr, ok := keyVal.(String)
		if !ok {
			panic("dictionary key did not evaluate to a string")
		}
		dict[keyStr] = ent.value.Eval(ec)
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

func (n *Unary) Eval(ec *EvalContext) Value {
	x := n.operand.Eval(ec)
	op := unaryops[unaryopKey{n.op, x.Kind()}]
	if op == nil {
		panic(fmt.Sprintf("unsupported unary operation: %s %s", n.op, x.Kind()))
	}
	return op(x)
}

func (n *Binary) Eval(ec *EvalContext) Value {
	left, right := n.left.Eval(ec), n.right.Eval(ec)
	op := binops[binopKey{n.op, left.Kind(), right.Kind()}]
	if op == nil {
		panic(fmt.Sprintf("unsupported binary operation: %s %s %s", left.Kind(), n.op, right.Kind()))
	}
	return op(left, right)
}

func (n *FieldAccess) Eval(ec *EvalContext) Value {
	panic("TODO: implement field access")
}

// Functions ///////////////////////////////////////////////////////////////////////////////////////

func (n *FuncCall) Eval(ec *EvalContext) Value {
	callee := n.callee.Eval(ec)
	fn, ok := callee.(*Function)
	if !ok {
		panic("attempted to call a non-function value")
	}

	var args Arguments
	for _, arg := range n.args {
		switch a := arg.(type) {
		case *ExprArg:
			args.Positional = append(args.Positional, a.expr.Eval(ec))
		case *NamedArg:
			if args.Named == nil {
				args.Named = make(map[unique.Handle[string]]Value)
			}
			args.Named[a.name] = a.expr.Eval(ec)
		case *SpreadArg:
			panic("TODO: implement spread arguments")
		default:
			panic("unrecognized function argument type")
		}
	}
	return fn.Apply(&args)
}

func (n *Closure) Eval(ec *EvalContext) Value {
	panic("TODO: not actually an expression")
}

// Bindings & Rules ////////////////////////////////////////////////////////////////////////////////

func (n *LetBinding) Eval(ec *EvalContext) Value {
	for _, p := range n.pattern {
		switch p := p.(type) {
		case *DestructIdent:
			ec.Bind(p.ident.name, n.value.Eval(ec))
		default:
			panic("TODO: implement complex let patterns")
		}
	}
	return &None{}
}

func (n *DestructAssignment) Eval(ec *EvalContext) Value {
	panic("TODO: implement destruct assignment")
}

func (n *SetRule) Eval(ec *EvalContext) Value {
	panic("TODO: implement set rules")
}

func (n *ShowRule) Eval(ec *EvalContext) Value {
	panic("TODO: implement show rules")
}

// Control Flow ////////////////////////////////////////////////////////////////////////////////////

func (n *Conditional) Eval(ec *EvalContext) Value {
	cond, ok := n.condition.Eval(ec).(Bool)
	if !ok {
		panic("condition did not evaluate to a boolean")
	}
	if cond {
		return n.then.Eval(ec)
	} else if n.els != nil {
		return n.els.Eval(ec)
	}
	return &None{}
}

func (n *ForLoop) Eval(ec *EvalContext) Value {
	panic("TODO: implement for loops")
}

func (n *WhileLoop) Eval(ec *EvalContext) Value {
	panic("TODO: implement while loops")
}

func (n *LoopBreak) Eval(ec *EvalContext) Value {
	panic("TODO: not actually an expression")
}

func (n *LoopContinue) Eval(ec *EvalContext) Value {
	panic("TODO: not actually an expression")
}

func (n *FuncReturn) Eval(ec *EvalContext) Value {
	panic("TODO: not actually an expression")
}

// Other ///////////////////////////////////////////////////////////////////////////////////////////

func (n *Contextual) Eval(ec *EvalContext) Value { panic("TODO: implement contextual") }

func (n *ModuleInclude) Eval(ec *EvalContext) Value { panic("TODO: implement module include") }
